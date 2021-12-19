
package chaincode

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

const commitmentCollection = "commitmentCollection"
const transferAgreementObjectType = "transferAgreement"

// SmartContract of this fabric sample
type SmartContract struct {
	contractapi.Contract
}

// Commitment describes main commitment details that are visible to all organizations
type Commitment struct {
	Type  string `json:"objectType"` //Type is used to distinguish the various types of objects in state database
	ID    string `json:"commitmentID"`
	Location string `json:"location"`
	Size  int    `json:"size"`
	Crop string `json:"crop"` 
	Owner string `json:"owner"`
}

// CommitmentPrivateDetails describes details that are private to owners
type CommitmentPrivateDetails struct {
	ID             string `json:"commitmentID"`
	Rate int    `json:"rate"`
}

// TransferAgreement describes the buyer agreement returned by ReadTransferAgreement
type TransferAgreement struct {
	ID      string `json:"commitmentID"`
	BuyerID string `json:"buyerID"`
}

// CreateCommitment creates a new commitment by placing the main commitment details in the commitmentCollection
// that can be read by both organizations. The appraisal value is stored in the owners org specific collection.
func (s *SmartContract) CreateCommitment(ctx contractapi.TransactionContextInterface) error {

	// Get new commitment from transient map
	transientMap, err := ctx.GetStub().GetTransient()
	if err != nil {
		return fmt.Errorf("error getting transient: %v", err)
	}

	// Commitment properties are private, therefore they get passed in transient field, instead of func args
	transientCommitmentJSON, ok := transientMap["commitment_properties"]
	if !ok {
		//log error to stdout
		return fmt.Errorf("commitment not found in the transient map input")
	}

	type commitmentTransientInput struct {
		Type           string `json:"objectType"` //Type is used to distinguish the various types of objects in state database
		ID             string `json:"commitmentID"`
		Location          string `json:"location"`
		Size           int    `json:"size"`
		Crop          string `json:"crop"`
		Rate int    `json:"rate"`
	}

	var commitmentInput commitmentTransientInput
	err = json.Unmarshal(transientCommitmentJSON, &commitmentInput)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	if len(commitmentInput.Type) == 0 {
		return fmt.Errorf("objectType field must be a non-empty string")
	}
	if len(commitmentInput.ID) == 0 {
		return fmt.Errorf("commitmentID field must be a non-empty string")
	}
	if len(commitmentInput.Location) == 0 {
		return fmt.Errorf("location field must be a non-empty string")
	}
	if commitmentInput.Size <= 0 {
		return fmt.Errorf("size field must be a positive integer")
	}
	if commitmentInput.Crop == 0 {
		return fmt.Errorf("crop field must be a non-empty string")
	}
	if commitmentInput.Rate <= 0 {
		return fmt.Errorf("rate field must be a positive integer")
	}

	// Check if commitment already exists
	commitmentAsBytes, err := ctx.GetStub().GetPrivateData(commitmentCollection, commitmentInput.ID)
	if err != nil {
		return fmt.Errorf("failed to get commitment: %v", err)
	} else if commitmentAsBytes != nil {
		fmt.Println("Commitment already exists: " + commitmentInput.ID)
		return fmt.Errorf("this commitment already exists: " + commitmentInput.ID)
	}

	// Get ID of submitting client identity
	clientID, err := submittingClientIdentity(ctx)
	if err != nil {
		return err
	}

	// Verify that the client is submitting request to peer in their organization
	// This is to ensure that a client from another org doesn't attempt to read or
	// write private data from this peer.
	err = verifyClientOrgMatchesPeerOrg(ctx)
	if err != nil {
		return fmt.Errorf("CreateCommitment cannot be performed: Error %v", err)
	}

	// Make submitting client the owner
	commitment := Commitment{
		Type:  commitmentInput.Type,
		ID:    commitmentInput.ID,
		Location: commitmentInput.Location,
		Size:  commitmentInput.Size,
		Crop: commitmentInput.Crop,
		Owner: clientID,
	}
	commitmentJSONasBytes, err := json.Marshal(commitment)
	if err != nil {
		return fmt.Errorf("failed to marshal commitment into JSON: %v", err)
	}

	// Save commitment to private data collection
	// Typical logger, logs to stdout/file in the fabric managed docker container, running this chaincode
	// Look for container name like dev-peer0.org1.example.com-{chaincodename_version}-xyz
	log.Printf("CreateCommitment Put: collection %v, ID %v, owner %v", commitmentCollection, commitmentInput.ID, clientID)

	err = ctx.GetStub().PutPrivateData(commitmentCollection, commitmentInput.ID, commitmentJSONasBytes)
	if err != nil {
		return fmt.Errorf("failed to put commitment into private data collecton: %v", err)
	}

	// Save commitment details to collection visible to owning organization
	commitmentPrivateDetails := CommitmentPrivateDetails{
		ID:             commitmentInput.ID,
		Rate: commitmentInput.Rate,
	}

	commitmentPrivateDetailsAsBytes, err := json.Marshal(commitmentPrivateDetails) // marshal commitment details to JSON
	if err != nil {
		return fmt.Errorf("failed to marshal into JSON: %v", err)
	}

	// Get collection name for this organization.
	orgCollection, err := getCollectionName(ctx)
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}

	// Put commitment appraised value into owners org specific private data collection
	log.Printf("Put: collection %v, ID %v", orgCollection, commitmentInput.ID)
	err = ctx.GetStub().PutPrivateData(orgCollection, commitmentInput.ID, commitmentPrivateDetailsAsBytes)
	if err != nil {
		return fmt.Errorf("failed to put commitment private details: %v", err)
	}
	return nil
}

// AgreeToTransfer is used by the potential buyer of the commitment to agree to the
// commitment value. The agreed to appraisal value is stored in the buying orgs
// org specifc collection, while the the buyer client ID is stored in the commitment collection
// using a composite key
func (s *SmartContract) AgreeToTransfer(ctx contractapi.TransactionContextInterface) error {

	// Get ID of submitting client identity
	clientID, err := submittingClientIdentity(ctx)
	if err != nil {
		return err
	}

	// Value is private, therefore it gets passed in transient field
	transientMap, err := ctx.GetStub().GetTransient()
	if err != nil {
		return fmt.Errorf("error getting transient: %v", err)
	}

	// Persist the JSON bytes as-is so that there is no risk of nondeterministic marshaling.
	valueJSONasBytes, ok := transientMap["commitment_value"]
	if !ok {
		return fmt.Errorf("commitment_value key not found in the transient map")
	}

	// Unmarshal the tranisent map to get the commitment ID.
	var valueJSON CommitmentPrivateDetails
	err = json.Unmarshal(valueJSONasBytes, &valueJSON)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	// Do some error checking since we get the chance
	if len(valueJSON.ID) == 0 {
		return fmt.Errorf("commitmentID field must be a non-empty string")
	}
	if valueJSON.Rate <= 0 {
		return fmt.Errorf("rate field must be a positive integer")
	}

	// Read commitment from the private data collection
	commitment, err := s.ReadCommitment(ctx, valueJSON.ID)
	if err != nil {
		return fmt.Errorf("error reading commitment: %v", err)
	}
	if commitment == nil {
		return fmt.Errorf("%v does not exist", valueJSON.ID)
	}
	// Verify that the client is submitting request to peer in their organization
	err = verifyClientOrgMatchesPeerOrg(ctx)
	if err != nil {
		return fmt.Errorf("AgreeToTransfer cannot be performed: Error %v", err)
	}

	// Get collection name for this organization. Needs to be read by a member of the organization.
	orgCollection, err := getCollectionName(ctx)
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}

	log.Printf("AgreeToTransfer Put: collection %v, ID %v", orgCollection, valueJSON.ID)
	// Put agreed value in the org specifc private data collection
	err = ctx.GetStub().PutPrivateData(orgCollection, valueJSON.ID, valueJSONasBytes)
	if err != nil {
		return fmt.Errorf("failed to put commitment bid: %v", err)
	}

	// Create agreeement that indicates which identity has agreed to purchase
	// In a more realistic transfer scenario, a transfer agreement would be secured to ensure that it cannot
	// be overwritten by another channel member
	transferAgreeKey, err := ctx.GetStub().CreateCompositeKey(transferAgreementObjectType, []string{valueJSON.ID})
	if err != nil {
		return fmt.Errorf("failed to create composite key: %v", err)
	}

	log.Printf("AgreeToTransfer Put: collection %v, ID %v, Key %v", commitmentCollection, valueJSON.ID, transferAgreeKey)
	err = ctx.GetStub().PutPrivateData(commitmentCollection, transferAgreeKey, []byte(clientID))
	if err != nil {
		return fmt.Errorf("failed to put commitment bid: %v", err)
	}

	return nil
}

// TransferCommitment transfers the commitment to the new owner by setting a new owner ID
func (s *SmartContract) TransferCommitment(ctx contractapi.TransactionContextInterface) error {

	transientMap, err := ctx.GetStub().GetTransient()
	if err != nil {
		return fmt.Errorf("error getting transient %v", err)
	}

	// Commitment properties are private, therefore they get passed in transient field
	transientTransferJSON, ok := transientMap["commitment_owner"]
	if !ok {
		return fmt.Errorf("commitment owner not found in the transient map")
	}

	type commitmentTransferTransientInput struct {
		ID       string `json:"commitmentID"`
		BuyerMSP string `json:"buyerMSP"`
	}

	var commitmentTransferInput commitmentTransferTransientInput
	err = json.Unmarshal(transientTransferJSON, &commitmentTransferInput)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	if len(commitmentTransferInput.ID) == 0 {
		return fmt.Errorf("commitmentID field must be a non-empty string")
	}
	if len(commitmentTransferInput.BuyerMSP) == 0 {
		return fmt.Errorf("buyerMSP field must be a non-empty string")
	}
	log.Printf("TransferCommitment: verify commitment exists ID %v", commitmentTransferInput.ID)
	// Read commitment from the private data collection
	commitment, err := s.ReadCommitment(ctx, commitmentTransferInput.ID)
	if err != nil {
		return fmt.Errorf("error reading commitment: %v", err)
	}
	if commitment == nil {
		return fmt.Errorf("%v does not exist", commitmentTransferInput.ID)
	}
	// Verify that the client is submitting request to peer in their organization
	err = verifyClientOrgMatchesPeerOrg(ctx)
	if err != nil {
		return fmt.Errorf("TransferCommitment cannot be performed: Error %v", err)
	}

	// Verify transfer details and transfer owner
	err = s.verifyAgreement(ctx, commitmentTransferInput.ID, commitment.Owner, commitmentTransferInput.BuyerMSP)
	if err != nil {
		return fmt.Errorf("failed transfer verification: %v", err)
	}

	transferAgreement, err := s.ReadTransferAgreement(ctx, commitmentTransferInput.ID)
	if err != nil {
		return fmt.Errorf("failed ReadTransferAgreement to find buyerID: %v", err)
	}
	if transferAgreement.BuyerID == "" {
		return fmt.Errorf("BuyerID not found in TransferAgreement for %v", commitmentTransferInput.ID)
	}

	// Transfer commitment in private data collection to new owner
	commitment.Owner = transferAgreement.BuyerID

	commitmentJSONasBytes, err := json.Marshal(commitment)
	if err != nil {
		return fmt.Errorf("failed marshalling commitment %v: %v", commitmentTransferInput.ID, err)
	}

	log.Printf("TransferCommitment Put: collection %v, ID %v", commitmentCollection, commitmentTransferInput.ID)
	err = ctx.GetStub().PutPrivateData(commitmentCollection, commitmentTransferInput.ID, commitmentJSONasBytes) //rewrite the commitment
	if err != nil {
		return err
	}

	// Get collection name for this organization
	ownersCollection, err := getCollectionName(ctx)
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}

	// Delete the commitment appraised value from this organization's private data collection
	err = ctx.GetStub().DelPrivateData(ownersCollection, commitmentTransferInput.ID)
	if err != nil {
		return err
	}

	// Delete the transfer agreement from the commitment collection
	transferAgreeKey, err := ctx.GetStub().CreateCompositeKey(transferAgreementObjectType, []string{commitmentTransferInput.ID})
	if err != nil {
		return fmt.Errorf("failed to create composite key: %v", err)
	}

	err = ctx.GetStub().DelPrivateData(commitmentCollection, transferAgreeKey)
	if err != nil {
		return err
	}

	return nil

}

// verifyAgreement is an internal helper function used by TransferCommitment to verify
// that the transfer is being initiated by the owner and that the buyer has agreed
// to the same appraisal value as the owner
func (s *SmartContract) verifyAgreement(ctx contractapi.TransactionContextInterface, commitmentID string, owner string, buyerMSP string) error {

	// Check 1: verify that the transfer is being initiatied by the owner

	// Get ID of submitting client identity
	clientID, err := submittingClientIdentity(ctx)
	if err != nil {
		return err
	}

	if clientID != owner {
		return fmt.Errorf("error: submitting client identity does not own commitment")
	}

	// Check 2: verify that the buyer has agreed to the appraised value

	// Get collection names
	collectionOwner, err := getCollectionName(ctx) // get owner collection from caller identity
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}

	collectionBuyer := buyerMSP + "PrivateCollection" // get buyers collection

	// Get hash of owners agreed to value
	ownerRateHash, err := ctx.GetStub().GetPrivateDataHash(collectionOwner, commitmentID)
	if err != nil {
		return fmt.Errorf("failed to get hash of appraised value from owners collection %v: %v", collectionOwner, err)
	}
	if ownerRateHash == nil {
		return fmt.Errorf("hash of appraised value for %v does not exist in collection %v", commitmentID, collectionOwner)
	}

	// Get hash of buyers agreed to value
	buyerRateHash, err := ctx.GetStub().GetPrivateDataHash(collectionBuyer, commitmentID)
	if err != nil {
		return fmt.Errorf("failed to get hash of appraised value from buyer collection %v: %v", collectionBuyer, err)
	}
	if buyerRateHash == nil {
		return fmt.Errorf("hash of appraised value for %v does not exist in collection %v. AgreeToTransfer must be called by the buyer first", commitmentID, collectionBuyer)
	}

	// Verify that the two hashes match
	if !bytes.Equal(ownerRateHash, buyerRateHash) {
		return fmt.Errorf("hash for appraised value for owner %x does not value for seller %x", ownerRateHash, buyerRateHash)
	}

	return nil
}

// DeleteCommitment can be used by the owner of the commitment to delete the commitment
func (s *SmartContract) DeleteCommitment(ctx contractapi.TransactionContextInterface) error {

	transientMap, err := ctx.GetStub().GetTransient()
	if err != nil {
		return fmt.Errorf("Error getting transient: %v", err)
	}

	// Commitment properties are private, therefore they get passed in transient field
	transientDeleteJSON, ok := transientMap["commitment_delete"]
	if !ok {
		return fmt.Errorf("commitment to delete not found in the transient map")
	}

	type commitmentDelete struct {
		ID string `json:"commitmentID"`
	}

	var commitmentDeleteInput commitmentDelete
	err = json.Unmarshal(transientDeleteJSON, &commitmentDeleteInput)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	if len(commitmentDeleteInput.ID) == 0 {
		return fmt.Errorf("commitmentID field must be a non-empty string")
	}

	// Verify that the client is submitting request to peer in their organization
	err = verifyClientOrgMatchesPeerOrg(ctx)
	if err != nil {
		return fmt.Errorf("DeleteCommitment cannot be performed: Error %v", err)
	}

	log.Printf("Deleting Commitment: %v", commitmentDeleteInput.ID)
	valAsbytes, err := ctx.GetStub().GetPrivateData(commitmentCollection, commitmentDeleteInput.ID) //get the commitment from chaincode state
	if err != nil {
		return fmt.Errorf("failed to read commitment: %v", err)
	}
	if valAsbytes == nil {
		return fmt.Errorf("commitment not found: %v", commitmentDeleteInput.ID)
	}

	ownerCollection, err := getCollectionName(ctx) // Get owners collection
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}

	//check the commitment is in the caller org's private collection
	valAsbytes, err = ctx.GetStub().GetPrivateData(ownerCollection, commitmentDeleteInput.ID)
	if err != nil {
		return fmt.Errorf("failed to read commitment from owner's Collection: %v", err)
	}
	if valAsbytes == nil {
		return fmt.Errorf("commitment not found in owner's private Collection %v: %v", ownerCollection, commitmentDeleteInput.ID)
	}

	// delete the commitment from state
	err = ctx.GetStub().DelPrivateData(commitmentCollection, commitmentDeleteInput.ID)
	if err != nil {
		return fmt.Errorf("failed to delete state: %v", err)
	}

	// Finally, delete private details of commitment
	err = ctx.GetStub().DelPrivateData(ownerCollection, commitmentDeleteInput.ID)
	if err != nil {
		return err
	}

	return nil

}

// DeleteTranferAgreement can be used by the buyer to withdraw a proposal from
// the commitment collection and from his own collection.
func (s *SmartContract) DeleteTranferAgreement(ctx contractapi.TransactionContextInterface) error {

	transientMap, err := ctx.GetStub().GetTransient()
	if err != nil {
		return fmt.Errorf("error getting transient: %v", err)
	}

	// Commitment properties are private, therefore they get passed in transient field
	transientDeleteJSON, ok := transientMap["agreement_delete"]
	if !ok {
		return fmt.Errorf("commitment to delete not found in the transient map")
	}

	type commitmentDelete struct {
		ID string `json:"commitmentID"`
	}

	var commitmentDeleteInput commitmentDelete
	err = json.Unmarshal(transientDeleteJSON, &commitmentDeleteInput)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	if len(commitmentDeleteInput.ID) == 0 {
		return fmt.Errorf("transient input ID field must be a non-empty string")
	}

	// Verify that the client is submitting request to peer in their organization
	err = verifyClientOrgMatchesPeerOrg(ctx)
	if err != nil {
		return fmt.Errorf("DeleteTranferAgreement cannot be performed: Error %v", err)
	}
	// Delete private details of agreement
	orgCollection, err := getCollectionName(ctx) // Get proposers collection.
	if err != nil {
		return fmt.Errorf("failed to infer private collection name for the org: %v", err)
	}
	tranferAgreeKey, err := ctx.GetStub().CreateCompositeKey(transferAgreementObjectType, []string{commitmentDeleteInput.
		ID}) // Create composite key
	if err != nil {
		return fmt.Errorf("failed to create composite key: %v", err)
	}

	valAsbytes, err := ctx.GetStub().GetPrivateData(commitmentCollection, tranferAgreeKey) //get the transfer_agreement
	if err != nil {
		return fmt.Errorf("failed to read transfer_agreement: %v", err)
	}
	if valAsbytes == nil {
		return fmt.Errorf("commitment's transfer_agreement does not exist: %v", commitmentDeleteInput.ID)
	}

	log.Printf("Deleting TranferAgreement: %v", commitmentDeleteInput.ID)
	err = ctx.GetStub().DelPrivateData(orgCollection, commitmentDeleteInput.ID) // Delete the commitment
	if err != nil {
		return err
	}

	// Delete transfer agreement record
	err = ctx.GetStub().DelPrivateData(commitmentCollection, tranferAgreeKey) // remove agreement from state
	if err != nil {
		return err
	}

	return nil

}

// getCollectionName is an internal helper function to get collection of submitting client identity.
func getCollectionName(ctx contractapi.TransactionContextInterface) (string, error) {

	// Get the MSP ID of submitting client identity
	clientMSPID, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", fmt.Errorf("failed to get verified MSPID: %v", err)
	}

	// Create the collection name
	orgCollection := clientMSPID + "PrivateCollection"

	return orgCollection, nil
}

// verifyClientOrgMatchesPeerOrg is an internal function used verify client org id and matches peer org id.
func verifyClientOrgMatchesPeerOrg(ctx contractapi.TransactionContextInterface) error {
	clientMSPID, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("failed getting the client's MSPID: %v", err)
	}
	peerMSPID, err := shim.GetMSPID()
	if err != nil {
		return fmt.Errorf("failed getting the peer's MSPID: %v", err)
	}

	if clientMSPID != peerMSPID {
		return fmt.Errorf("client from org %v is not authorized to read or write private data from an org %v peer", clientMSPID, peerMSPID)
	}

	return nil
}

func submittingClientIdentity(ctx contractapi.TransactionContextInterface) (string, error) {
	b64ID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("Failed to read clientID: %v", err)
	}
	decodeID, err := base64.StdEncoding.DecodeString(b64ID)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode clientID: %v", err)
	}
	return string(decodeID), nil
}

package chaincode

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// ReadCommitment reads the information from collection
func (s *SmartContract) ReadCommitment(ctx contractapi.TransactionContextInterface, commitmentID string) (*Commitment, error) {

	log.Printf("ReadCommitment: collection %v, ID %v", commitmentCollection, commitmentID)
	commitmentJSON, err := ctx.GetStub().GetPrivateData(commitmentCollection, commitmentID) //get the commitment from chaincode state
	if err != nil {
		return nil, fmt.Errorf("failed to read commitment: %v", err)
	}

	//No Commitment found, return empty response
	if commitmentJSON == nil {
		log.Printf("%v does not exist in collection %v", commitmentID, commitmentCollection)
		return nil, nil
	}

	var commitment *Commitment
	err = json.Unmarshal(commitmentJSON, &commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	return commitment, nil

}

func (s *SmartContract) ReadProduced(ctx contractapi.TransactionContextInterface, yieldID string) (*Yield, error){

	log.Printf("ReadYield: collection %v, ID %v", yieldCollection, yieldID)
	yieldJSON, err := ctx.GetStub().GetPrivateData(yieldCollection, yieldID) //get the commitment from chaincode state
	if err != nil {
		return nil, fmt.Errorf("failed to read yield: %v", err)
	}

	//No Commitment found, return empty response
	if yieldJSON == nil {
		log.Printf("%v does not exist in collection %v", yieldID, yieldCollection)
		return nil, nil
	}

	var yield *Yield
	err = json.Unmarshal(yieldJSON, &yield)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	return yield, nil
}

func (s *SmartContract) ReadData(ctx contractapi.TransactionContextInterface, dataID string) (*Data, error) {
	
	log.Printf("ReadData: collection %v, ID %v", dataCollection, dataID)
	dataJSON, err := ctx.GetStub().GetPrivateData(dataCollection, dataID) //get the commitment from chaincode state
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %v", err)
	}

	//No Commitment found, return empty response
	if dataJSON == nil {
		log.Printf("%v does not exist in collection %v", dataID, dataCollection)
		return nil, nil
	}

	var data *Data
	err = json.Unmarshal(dataJSON, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	return data, nil
}


// ReadCommitmentPrivateDetails reads the commitment private details in organization specific collection
func (s *SmartContract) ReadCommitmentPrivateDetails(ctx contractapi.TransactionContextInterface, collection string, commitmentID string) (*CommitmentPrivateDetails, error) {
	log.Printf("ReadCommitmentPrivateDetails: collection %v, ID %v", collection, commitmentID)
	commitmentDetailsJSON, err := ctx.GetStub().GetPrivateData(collection, commitmentID) // Get the commitment from chaincode state
	if err != nil {
		return nil, fmt.Errorf("failed to read commitment details: %v", err)
	}
	if commitmentDetailsJSON == nil {
		log.Printf("CommitmentPrivateDetails for %v does not exist in collection %v", commitmentID, collection)
		return nil, nil
	}

	var commitmentDetails *CommitmentPrivateDetails
	err = json.Unmarshal(commitmentDetailsJSON, &commitmentDetails)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	return commitmentDetails, nil
}

// ReadTransferAgreement gets the buyer's identity from the transfer agreement from collection
func (s *SmartContract) ReadTransferAgreement(ctx contractapi.TransactionContextInterface, commitmentID string) (*TransferAgreement, error) {
	log.Printf("ReadTransferAgreement: collection %v, ID %v", commitmentCollection, commitmentID)
	// composite key for TransferAgreement of this commitment
	transferAgreeKey, err := ctx.GetStub().CreateCompositeKey(transferAgreementObjectType, []string{commitmentID})
	if err != nil {
		return nil, fmt.Errorf("failed to create composite key: %v", err)
	}

	buyerIdentity, err := ctx.GetStub().GetPrivateData(commitmentCollection, transferAgreeKey) // Get the identity from collection
	if err != nil {
		return nil, fmt.Errorf("failed to read TransferAgreement: %v", err)
	}
	if buyerIdentity == nil {
		log.Printf("TransferAgreement for %v does not exist", commitmentID)
		return nil, nil
	}
	agreement := &TransferAgreement{
		ID:      commitmentID,
		BuyerID: string(buyerIdentity),
	}
	return agreement, nil
}

// GetCommitmentByRange performs a range query based on the start and end keys provided. Range
// queries can be used to read data from private data collections, but can not be used in
// a transaction that also writes to private data.
func (s *SmartContract) GetCommitmentByRange(ctx contractapi.TransactionContextInterface, startKey string, endKey string) ([]*Commitment, error) {

	resultsIterator, err := ctx.GetStub().GetPrivateDataByRange(commitmentCollection, startKey, endKey)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	results := []*Commitment{}

	for resultsIterator.HasNext() {
		response, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var commitment *Commitment
		err = json.Unmarshal(response.Value, &commitment)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
		}

		results = append(results, commitment)
	}

	return results, nil

}

// =======Rich queries =========================================================================
// Two examples of rich queries are provided below (parameterized query and ad hoc query).
// Rich queries pass a query string to the state database.
// Rich queries are only supported by state database implementations
//  that support rich query (e.g. CouchDB).
// The query string is in the syntax of the underlying state database.
// With rich queries there is no guarantee that the result set hasn't changed between
//  endorsement time and commit time, aka 'phantom reads'.
// Therefore, rich queries should not be used in update transactions, unless the
// application handles the possibility of result set changes between endorsement and commit time.
// Rich queries can be used for point-in-time queries against a peer.
// ============================================================================================

// ===== Example: Parameterized rich query =================================================

// QueryCommitmentByOwner queries for commitments based on commitmentType, owner.
// This is an example of a parameterized query where the query logic is baked into the chaincode,
// and accepting a single query parameter (owner).
// Only available on state databases that support rich query (e.g. CouchDB)
// =========================================================================================
func (s *SmartContract) QueryCommitmentByOwner(ctx contractapi.TransactionContextInterface, commitmentType string, owner string) ([]*Commitment, error) {

	queryString := fmt.Sprintf("{\"selector\":{\"objectType\":\"%v\",\"owner\":\"%v\"}}", commitmentType, owner)

	queryResults, err := s.getQueryResultForQueryString(ctx, queryString)
	if err != nil {
		return nil, err
	}
	return queryResults, nil
}

// QueryCommitments uses a query string to perform a query for commitments.
// Query string matching state database syntax is passed in and executed as is.
// Supports ad hoc queries that can be defined at runtime by the client.
// If this is not desired, follow the QueryCommitmentByOwner example for parameterized queries.
// Only available on state databases that support rich query (e.g. CouchDB)
func (s *SmartContract) QueryCommitments(ctx contractapi.TransactionContextInterface, queryString string) ([]*Commitment, error) {

	queryResults, err := s.getQueryResultForQueryString(ctx, queryString)
	if err != nil {
		return nil, err
	}
	return queryResults, nil
}

// getQueryResultForQueryString executes the passed in query string.
func (s *SmartContract) getQueryResultForQueryString(ctx contractapi.TransactionContextInterface, queryString string) ([]*Commitment, error) {

	resultsIterator, err := ctx.GetStub().GetPrivateDataQueryResult(commitmentCollection, queryString)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	results := []*Commitment{}

	for resultsIterator.HasNext() {
		response, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}
		var commitment *Commitment

		err = json.Unmarshal(response.Value, &commitment)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
		}

		results = append(results, commitment)
	}
	return results, nil
}

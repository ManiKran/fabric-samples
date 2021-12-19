/*
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/hyperledger/fabric-samples/yield-commitment/chaincode-go/chaincode"
)

func main() {
	commitmentChaincode, err := contractapi.NewChaincode(&chaincode.SmartContract{})
	if err != nil {
		log.Panicf("Error creating yield-commitment chaincode: %v", err)
	}

	if err := commitmentChaincode.Start(); err != nil {
		log.Panicf("Error starting yield-commitment chaincode: %v", err)
	}
}

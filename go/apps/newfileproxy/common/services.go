// Copyright (c) 2014, Google, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// File: services.go

package common;

import (
	"errors"
	"fmt"

	// "github.com/jlmucb/cloudproxy/go/tao"
	// "github.com/jlmucb/cloudproxy/go/tao/auth"
	"github.com/golang/protobuf/proto"
	"github.com/jlmucb/cloudproxy/go/util"
	"github.com/jlmucb/cloudproxy/go/apps/simpleexample/taosupport"
	"github.com/jlmucb/cloudproxy/go/apps/newfileproxy/resourcemanager"
)

func stringIntoPointer(s1 string) *string {
        return &s1
}

func intIntoPointer(i int) *int32 {
	ii := int32(i)
        return &ii
}

// SendFile reads a file from disk and streams it to a receiver across a
// MessageStream. 
func SendFile(ms *util.MessageStream, m *resourcemanager.ResourceMasterInfo,
		info *resourcemanager.ResourceInfo) error {
	fileContents, err := info.Read(*m.BaseDirectoryName)
	if err != nil {
		return errors.New("No message payload")
	}
	fmt.Printf("File contents: %x\n", fileContents)
	var outerMessage taosupport.SimpleMessage
	outerMessage.MessageType = intIntoPointer(int(taosupport.MessageType_RESPONSE))
	var msg FileproxyMessage
	msgtype := MessageType(MessageType_READ)
	msg.Type = &msgtype
	msg.NumTotalBuffers = intIntoPointer(1)
	msg.CurrentBuffer = intIntoPointer(1)
	msg.Data[0] = fileContents
	var payload []byte
	payload, err = proto.Marshal(&msg)
	outerMessage.Data[0] = payload
	return taosupport.SendMessage(ms, &outerMessage)
}

// GetFile receives bytes from a sender and optionally encrypts them and adds
// integrity protection, and writes them to disk.
func GetFile(ms *util.MessageStream, m *resourcemanager.ResourceMasterInfo,
		info *resourcemanager.ResourceInfo) error {
	// Read bytes from channel
	outerMessage, err := taosupport.GetMessage(ms)
	if err != nil {
		return errors.New("Bad message")
	}
	if *outerMessage.MessageType != *intIntoPointer(int(taosupport.MessageType_RESPONSE)) {
		return errors.New("Bad message")
	}
	if len(outerMessage.Data) == 0 {
		return errors.New("No message payload")
	}
	var msg FileproxyMessage
	err = proto.Unmarshal(outerMessage.Data[0], &msg)
	if err != nil {
		return errors.New("Bad payload message")
	}
	if msg.Type == nil || *msg.Type != MessageType_READ {
		return errors.New("Wrong message response")
	}
	if len(msg.Data[0]) == 0 {
		return errors.New("No file contents")
	}
	fileContents := msg.Data[0]
	return info.Write(*m.BaseDirectoryName, fileContents)
}

type AuthentictedPrincipals struct {
	ValidPrincipals []resourcemanager.CombinedPrincipal
};

func FailureResponse(ms *util.MessageStream, msgType int, err_string string) {
	var outerMessage taosupport.SimpleMessage
	outerMessage.MessageType = intIntoPointer(msgType)
	outerMessage.Err = stringIntoPointer(err_string)
	taosupport.SendMessage(ms, &outerMessage)
	return
}

func IsAuthorized(action MessageType, resourceInfo *resourcemanager.ResourceInfo,
		policyKey []byte, principals* AuthentictedPrincipals) bool {
	switch(action) {
	default:
		return false
	case MessageType_REQUEST_CHALLENGE:
		return false
	case MessageType_CREATE:
		return false
	case MessageType_DELETE, MessageType_ADDWRITER, MessageType_DELETEWRITER, MessageType_WRITE:
		for p := range principals.ValidPrincipals {
			if resourceInfo.IsOwner(principals.ValidPrincipals[p]) || resourceInfo.IsWriter(principals.ValidPrincipals[p]) {
				return true
			}
		}
		return false
	case MessageType_ADDREADER, MessageType_DELETEREADER, MessageType_READ:
		for p := range principals.ValidPrincipals {
			if resourceInfo.IsOwner(principals.ValidPrincipals[p]) || resourceInfo.IsReader(principals.ValidPrincipals[p]) {
				return true
			}
		}
		return false
	case MessageType_ADDOWNER, MessageType_DELETEOWNER:
		for p := range principals.ValidPrincipals {
			if resourceInfo.IsOwner(principals.ValidPrincipals[p]) {
				return true
			}
		}
		return false
	}
}

func RequestChallenge(ms *util.MessageStream, cert []byte) error {
	var outerMessage taosupport.SimpleMessage
	outerMessage.MessageType = intIntoPointer(int(taosupport.MessageType_REQUEST))
	var msg FileproxyMessage
	msgType := MessageType_REQUEST_CHALLENGE
	msg.Type = &msgType
	msg.Data[0] = cert
	var payload []byte
	payload, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	outerMessage.Data[0] = payload
	return taosupport.SendMessage(ms, &outerMessage)
}

func Create(ms *util.MessageStream, name string, cert []byte) error {
	var outerMessage taosupport.SimpleMessage
	outerMessage.MessageType = intIntoPointer(int(taosupport.MessageType_REQUEST))
	var msg FileproxyMessage
	msgType := MessageType_CREATE
	msg.Type = &msgType
	msg.Arguments[0] = name
	msg.Data[0] = cert
	var payload []byte;
	payload, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	outerMessage.Data[0] = payload
	return taosupport.SendMessage(ms, &outerMessage)
}

func Delete(ms *util.MessageStream, name string) error {
	return nil
}

func AddOwner(ms *util.MessageStream, certs [][]byte) error {
	return nil
}

func AddReader(ms *util.MessageStream, certs [][]byte) error {
	return nil
}

func AddWriter(ms *util.MessageStream, certs [][]byte, nonce []byte) error {
	return nil
}

func DeleteOwner(ms *util.MessageStream, certs [][]byte) error {
	return nil
}

func DeleteReader(ms *util.MessageStream, certs [][]byte) error {
	return nil
}

func DeleteWriter(ms *util.MessageStream, certs [][]byte) error {
	return nil
}

func ReadResource(ms *util.MessageStream, resourceName string) error {
	return nil
}

func WriteResource(ms *util.MessageStream, resourceName string) error {
	return nil
}

func DoVerifyChallenge(ms *util.MessageStream, policyKey []byte, resourceMaster *resourcemanager.ResourceMasterInfo,
		principals *AuthentictedPrincipals, msg FileproxyMessage) bool {
	return false
}

// This is actually done by the server.
func DoChallenge(ms *util.MessageStream, policyKey []byte, resourceMaster *resourcemanager.ResourceMasterInfo,
                principals *AuthentictedPrincipals, msg FileproxyMessage) error {
	// Construct challenge
	// Send it
	// Get response
	// Should be a VERIFY_CHALLENGE response
	// If it verifies, put on authenticated principals
	// Send response
	return nil
}

func DoCreate(ms *util.MessageStream, policyKey []byte, resourceMaster *resourcemanager.ResourceMasterInfo,
                principals* AuthentictedPrincipals, msg FileproxyMessage) {
	// Authorized?
	// Put it in table.
	// Send response
	return
}

func DoDelete(ms *util.MessageStream, policyKey []byte, resourceMaster *resourcemanager.ResourceMasterInfo,
                principals* AuthentictedPrincipals, msg FileproxyMessage) {
	// Authorized?
	// Remove it from table.
	// Send response
	return
}

func DoAddOwner(ms *util.MessageStream, policyKey []byte, resourceMaster *resourcemanager.ResourceMasterInfo,
                principals* AuthentictedPrincipals, msg FileproxyMessage) {
	// Authorized?
	// Add it
	// Send response
	return
}

func DoAddReader(ms *util.MessageStream, policyKey []byte, resourceMaster *resourcemanager.ResourceMasterInfo,
                principals* AuthentictedPrincipals, msg FileproxyMessage) {
	return
}

func DoAddWriter(ms *util.MessageStream, policyKey []byte, resourceMaster *resourcemanager.ResourceMasterInfo,
                principals* AuthentictedPrincipals, msg FileproxyMessage) {
	return
}

func DoDeleteOwner(ms *util.MessageStream, policyKey []byte, resourceMaster *resourcemanager.ResourceMasterInfo,
                principals* AuthentictedPrincipals, msg FileproxyMessage) {
	return
}

func DoDeleteReader(ms *util.MessageStream, policyKey []byte, resourceMaster *resourcemanager.ResourceMasterInfo,
                principals* AuthentictedPrincipals, msg FileproxyMessage) {
	return
}

func DoDeleteWriter(ms *util.MessageStream, policyKey []byte, resourceMaster *resourcemanager.ResourceMasterInfo,
                principals* AuthentictedPrincipals, msg FileproxyMessage) {
	return
}

func DoReadResource(ms *util.MessageStream, policyKey []byte, m *resourcemanager.ResourceMasterInfo,
                principals* AuthentictedPrincipals, msg FileproxyMessage) {

	resourceName := msg.Arguments[0]
	info := m.FindResource(resourceName)
	if info == nil {
		FailureResponse(ms, int(MessageType_READ), "no such resource")
		return
	}
	if !IsAuthorized(*msg.Type, info, policyKey, principals) {
		FailureResponse(ms, int(MessageType_READ), "not authorized")
		return
	}
	// Send file
	_ = SendFile(ms, m, info)
	return
}

func DoWriteResource(ms *util.MessageStream, policyKey []byte, m *resourcemanager.ResourceMasterInfo,
                principals* AuthentictedPrincipals, msg FileproxyMessage) {
	resourceName := msg.Arguments[0]
	info := m.FindResource(resourceName)
	if info == nil {
		FailureResponse(ms, int(MessageType_WRITE), "no such resource")
		return
	}
	if !IsAuthorized(*msg.Type, info, policyKey, principals) {
		FailureResponse(ms, int(MessageType_WRITE), "not authorized")
		return
	}
	// Send file
	_ = SendFile(ms, m, info)
	return
}

// Dispatch

func DoRequest(ms *util.MessageStream, policyKey []byte,
		resourceMaster *resourcemanager.ResourceMasterInfo,
		principals* AuthentictedPrincipals, req []byte) {
	msg := new(FileproxyMessage);
	err := proto.Unmarshal(req, msg)
	if err != nil {
		return
	}
	switch(*msg.Type) {
	default:
		resp := new(taosupport.SimpleMessage)
		mt := int32(taosupport.MessageType_RESPONSE)
		resp.MessageType = &mt
		errString := "Unsupported request"
		resp.Err = &errString
		taosupport.SendMessage(ms, resp)
		return
	case MessageType_REQUEST_CHALLENGE:
		DoChallenge(ms, policyKey, resourceMaster, principals, *msg)
		return
	case MessageType_CREATE:
		DoCreate(ms, policyKey, resourceMaster, principals, *msg)
		return
	case MessageType_DELETE:
		DoDelete(ms, policyKey, resourceMaster, principals, *msg)
		return
	case MessageType_ADDREADER:
		DoAddReader(ms, policyKey, resourceMaster, principals, *msg)
		return
	case MessageType_ADDOWNER:
		DoAddOwner(ms, policyKey, resourceMaster, principals, *msg)
		return
	case MessageType_ADDWRITER:
		DoAddWriter(ms, policyKey, resourceMaster, principals, *msg)
		return
	case MessageType_DELETEREADER:
		DoDeleteReader(ms, policyKey, resourceMaster, principals, *msg)
		return
	case MessageType_DELETEOWNER:
		DoDeleteOwner(ms, policyKey, resourceMaster, principals, *msg)
		return
	case MessageType_DELETEWRITER:
		DoDeleteWriter(ms, policyKey, resourceMaster, principals, *msg)
		return
	case MessageType_READ:
		DoReadResource(ms, policyKey, resourceMaster, principals, *msg)
		return
	case MessageType_WRITE:
		DoWriteResource(ms, policyKey, resourceMaster, principals, *msg)
		return
	}
}


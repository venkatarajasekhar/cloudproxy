// Copyright (c) 2016, Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secret_service

// This class provides helper functions for the secret service.
//

import (
	"container/list"
	"crypto/rand"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/golang/protobuf/proto"
	"github.com/jlmucb/cloudproxy/go/support_libraries/protected_objects"
	"github.com/jlmucb/cloudproxy/go/support_libraries/secret_disclosure_support"
	"github.com/jlmucb/cloudproxy/go/tao"
	"github.com/jlmucb/cloudproxy/go/tao/auth"
)

// This struct is for storing server data, which includes the protected objects
// and ACLS for them.
type ServerData struct {
	// Protected object ACLs are stored in Domain.Guard.
	Domain *tao.Domain
	// EncKey is used for encrypting the root protected object.
	EncKey *tao.Keys
	// List of protected objects.
	Lis *list.List
	// The root protected object, which is a key used to encrypt other protected objects.
	RootPObj *protected_objects.ProtectedObjectMessage
	Path     *string
}

// This function initilizes a new server by creating a new ServerData struct based on
// the configuration it finds at configPath. A sample configuration can be found at
// ./server/tao.config. This struct is saved to disk in the $DIR/state/ directory where $DIR
// is the directory of configPath.
// Secrets in this ServerData struct are stored with domainPass as password.
// The certificate for the secret service contains secretServiceName as OrganizationalUnit
// and serviceHost as the CommonName (which is expected to be the hostname of the address at
// which the secret service is available)
func InitState(configPath, domainPass, secretServiceName, serviceHost string) (*ServerData, error) {
	dir := path.Join(path.Dir(configPath), "state")
	domain, err := createDomain(configPath, dir, domainPass)
	if err != nil {
		return nil, err
	}
	encKey, err := createEncKey(path.Join(dir, "encKey"), domainPass,
		secretServiceName, serviceHost, domain)
	if err != nil {
		return nil, err
	}
	pObj, err := createRoot(encKey, "Root", int32(0))
	if err != nil {
		return nil, err
	}
	objFile := path.Join(dir, "ProtectedObjects")
	rootFile := path.Join(dir, "RootProtectedObject")
	l := list.New()
	l.PushFront(*pObj)
	root := list.New()
	root.PushFront(*pObj)
	err = protected_objects.SaveProtectedObjects(l, objFile)
	if err != nil {
		log.Printf("Error saving list of objects to file %s", objFile)
		return nil, err
	}
	err = protected_objects.SaveProtectedObjects(root, rootFile)
	if err != nil {
		log.Printf("Error saving root to file %s", rootFile)
		return nil, err
	}
	state := ServerData{
		Domain:   domain,
		EncKey:   encKey,
		Lis:      l,
		RootPObj: pObj,
	}
	return &state, nil
}

// This function loads a ServerData struct from disk, saved with domainPass as password.
// It expects configPath to be the path of the server configuration file used to initialize the
// server.
func LoadState(configPath, domainPass string) (*ServerData, error) {
	configDir := path.Dir(configPath)
	configPath = path.Join(configDir, "state/server.config")
	_, err := os.Stat(configPath)
	if err != nil {
		return nil, errors.New(
			"Could not find server state to load. If starting first time, use -init flag.")
	}
	domain, err := tao.LoadDomain(configPath, []byte(domainPass))
	if domain == nil {
		log.Printf("secretserver: no domain path - %s, pass - %s, err - %s\n",
			configPath, domainPass, errors.New("nil domain loaded"))
		return nil, err
	} else if err != nil {
		log.Printf("secretserver: Couldn't load the config path %s: %s\n",
			configPath, err)
		return nil, err
	}
	configDir = path.Dir(configPath)
	encKeyPath := path.Join(configDir, "encKey")
	encKey, err := tao.NewOnDiskPBEKeys(tao.Crypting|tao.Signing, []byte(domainPass),
		encKeyPath, nil)
	if err != nil {
		log.Printf("secretserver: Couldn't load the enc key path %s: %s\n",
			encKeyPath, err)
		return nil, err
	}
	if encKey.Cert == nil {
		errStr := "secretserver: cert missing in loaded enc key."
		log.Printf(errStr)
		return nil, errors.New(errStr)
	}
	objFile := path.Join(configDir, "ProtectedObjects")
	rootFile := path.Join(configDir, "RootProtectedObject")

	l := protected_objects.LoadProtectedObjects(objFile)
	if l == nil {
		errStr := "secretserver: error in loading protected objects from disk."
		log.Printf(errStr)
		return nil, errors.New(errStr)
	}
	rootL := protected_objects.LoadProtectedObjects(rootFile)
	if rootL == nil {
		errStr := "secretserver: error in loading root protected object from disk."
		log.Printf(errStr)
		return nil, errors.New(errStr)
	}
	elem := rootL.Front()
	if elem == nil {
		errStr := "secretserver: error in loading root protected object from disk."
		log.Printf(errStr)
		return nil, errors.New(errStr)
	}
	pObj := elem.Value.(protected_objects.ProtectedObjectMessage)
	state := ServerData{
		Domain:   domain,
		EncKey:   encKey,
		Lis:      l,
		RootPObj: &pObj,
	}
	return &state, nil
}

// This function saves the ServerData object to disk in the $DIR/state/ directory where $DIR
// is the directory of the server configuration file used to initialize this server.
func SaveState(state *ServerData) error {
	if state.Domain != nil {
		err := state.Domain.Save()
		if err != nil {
			log.Printf("Error saving domain to path %s", state.Domain.ConfigPath)
			return err
		}
	}
	configDir := path.Dir(state.Domain.ConfigPath)
	objFile := path.Join(configDir, "ProtectedObjects")
	rootFile := path.Join(configDir, "RootProtectedObject")
	err := protected_objects.SaveProtectedObjects(state.Lis, objFile)
	if err != nil {
		log.Printf("Error saving list of objects to file %s", objFile)
		return err
	}
	l := list.New()
	l.PushFront(*state.RootPObj)
	err = protected_objects.SaveProtectedObjects(l, rootFile)
	if err != nil {
		log.Printf("Error saving root to file %s", rootFile)
		return err
	}
	return nil
}

// This function returns the type and value of a protected object identified by id, from
// the list of protected objects l whose root key is protected by encKey. It does so only
// when program is authorized to read the object as per the policy in domain.
func ReadObject(l *list.List, encKey *tao.Keys, id *protected_objects.ObjectIdMessage,
	program *auth.Prin, domain *tao.Domain) (*string, []byte, error) {

	if !domain.Guard.IsAuthorized(*program, secret_disclosure.ReadPredicate,
		[]string{id.String()}) {
		return nil, nil, errors.New("program not authorized to read requested secret")
	}
	return readObjRec(l, encKey, id)
}

func readObjRec(l *list.List, encKey *tao.Keys, id *protected_objects.ObjectIdMessage) (*string,
	[]byte, error) {

	elem := protected_objects.FindElementById(l, *id.ObjName, *id.ObjEpoch)
	if elem == nil {
		return nil, nil, errors.New("object not found")
	}
	pObj := elem.Value.(protected_objects.ProtectedObjectMessage)
	if pObj.ProtectorObjId == nil {
		// Decrypt root using encKeys.
		rootKey, err := encKey.CryptingKey.Decrypt(pObj.GetBlob())
		if err != nil {
			return nil, nil, err
		}
		str := "key"
		return &str, rootKey, nil
	}
	parentType, parentKey, err := readObjRec(l, encKey, pObj.ProtectorObjId)
	if err != nil {
		return nil, nil, err
	}
	if *parentType != "key" {
		return nil, nil, errors.New("internal node with type not key")
	}
	obj, err := protected_objects.RecoverProtectedObject(&pObj, parentKey)
	if err != nil {
		return nil, nil, err
	}
	return obj.ObjType, obj.ObjVal, nil
}

// This function writes newType and newVal as type and value to a protected object identified
// by id, where l is the list of protected objects and encKey protects the root key.
// The write is only performed if program is authorized to write the object as per policy in
// domain.
func WriteObject(l *list.List, encKey *tao.Keys, id *protected_objects.ObjectIdMessage,
	program *auth.Prin, domain *tao.Domain, newType string,
	newVal []byte) error {

	if !domain.Guard.IsAuthorized(*program, secret_disclosure.WritePredicate,
		[]string{id.String()}) {
		return errors.New("program not authorized to write requested secret")
	}

	element := protected_objects.FindElementById(l, *id.ObjName, *id.ObjEpoch)
	if element == nil {
		return errors.New("attemtping to write non-existant object")
	}
	pOld := element.Value.(protected_objects.ProtectedObjectMessage)
	parentId := pOld.ProtectorObjId
	if parentId == nil {
		return errors.New("attempting to write root key")
	}
	parentType, parentKey, err := readObjRec(l, encKey, parentId)
	if err != nil {
		return err
	}
	if *parentType != "key" {
		return errors.New("parent of object to be written is not a key")
	}
	new := protected_objects.ObjectMessage{
		ObjId:   id,
		ObjVal:  newVal,
		ObjType: &newType}
	pNew, err := protected_objects.MakeProtectedObject(new, *parentId.ObjName,
		*parentId.ObjEpoch, parentKey)
	if err != nil {
		return errors.New("can not make protected object")
	}
	element.Value = *pNew
	return nil
}

// This function creates a new protected object identified by newId, with newType and newVal
// as type and value respectively. The new object is protected by protected object identified
// by protectorId, which must be of type 'key'. The function adds this new object to the list
// l whose root key is protected by encKey. These operations are performed only when the
// program is authorized to create protected objects under protectorId as per the policy in
// domain.
func CreateObject(l *list.List, newId, protectorId, rootId *protected_objects.ObjectIdMessage,
	encKey *tao.Keys, program *auth.Prin, domain *tao.Domain, newType string,
	newVal []byte) error {
	if protectorId == nil {
		if !domain.Guard.IsAuthorized(*program, secret_disclosure.CreatePredicate,
			[]string{}) {
			return errors.New(fmt.Sprintf(
				"program %v not authorized to create requested secret under root",
				*program))
		}
	} else {
		if !domain.Guard.IsAuthorized(*program, secret_disclosure.CreatePredicate,
			[]string{protectorId.String()}) {
			return errors.New(fmt.Sprintf(
				"program %v not authorized to create requested secret under %v",
				*program, protectorId.String()))
		}
	}
	pId := protectorId
	if protectorId == nil {
		pId = rootId
	}
	_, _, err := readObjRec(l, encKey, newId)
	if err == nil {
		return errors.New("creating object with existing id")
	}

	protectorType, protectorKey, err := readObjRec(l, encKey, pId)
	if err != nil {
		return err
	}
	if *protectorType != "key" {
		return errors.New("creating object protected by object type not key")
	}

	new := protected_objects.ObjectMessage{
		ObjId:   newId,
		ObjVal:  newVal,
		ObjType: &newType}
	pNew, err := protected_objects.MakeProtectedObject(new, *pId.ObjName,
		*pId.ObjEpoch, protectorKey)

	l.PushFront(*pNew)
	err = domain.Guard.Authorize(*program, secret_disclosure.OwnPredicate, []string{newId.String()})
	if err != nil {
		return err
	}
	return nil
}

// This function deletes a protected object identified by id, from the list l. It does so only
// when program is authorized to delete the object as per the policy in domain.
func DeleteObject(l *list.List, id *protected_objects.ObjectIdMessage, program *auth.Prin,
	domain *tao.Domain) error {

	if !domain.Guard.IsAuthorized(*program, secret_disclosure.DeletePredicate,
		[]string{id.String()}) {
		return errors.New("program not authorized to delete requested secret")
	}

	element := protected_objects.FindElementById(l, *id.ObjName, *id.ObjEpoch)
	if element == nil {
		return errors.New("object to be deleted not found")
	}
	l.Remove(element)
	return nil
}

func createDomain(domainConfigPath, stateDir, domainPass string) (*tao.Domain, error) {
	var cfg tao.DomainConfig
	newConfigPath := path.Join(stateDir, "server.config")
	d, err := ioutil.ReadFile(domainConfigPath)
	if err != nil {
		log.Printf("secret server: error in reading domain config. err: %s, path: %s\n",
			err, domainConfigPath)
		return nil, err
	}
	if err := proto.UnmarshalText(string(d), &cfg); err != nil {
		log.Printf("secret server: error in parsing domain config. err: %s\n", err)
		return nil, err
	}
	domain, err := tao.CreateDomain(cfg, newConfigPath, []byte(domainPass))
	if domain == nil {
		log.Printf("secretserver: no domain path - %s, pass - %s, err - %s\n",
			domainConfigPath, domainPass, err)
		return nil, errors.New("nil domain created")
	} else if err != nil {
		log.Printf("secretserver: Couldn't load the config path %s: %s\n",
			domainConfigPath, err)
		return nil, err
	}
	err = domain.Save()
	if err != nil {
		log.Printf("secret server: error in saving domain. err: %s\n", err)
		return nil, err
	}
	return domain, nil
}

func createEncKey(encKeyPath, domainPass, secretServiceName string, serviceHost string,
	domain *tao.Domain) (*tao.Keys, error) {
	name := tao.NewX509Name(domain.Config.X509Info)
	name.OrganizationalUnit = []string{secretServiceName}
	name.CommonName = serviceHost
	var err error
	encKey, err := tao.NewSignedOnDiskPBEKeys(tao.Crypting|tao.Signing, []byte(domainPass),
		encKeyPath, name, 1, domain.Keys)
	if err != nil {
		log.Printf("secretserver: Couldn't load the create enc keys. path: %s, err: %s\n",
			encKeyPath, err)
		return nil, err
	}
	return encKey, nil
}

func createRoot(encKey *tao.Keys, rootName string,
	epoch int32) (*protected_objects.ProtectedObjectMessage, error) {
	rootKey := make([]byte, 32)
	_, err := rand.Read(rootKey)
	if err != nil {
		log.Printf("secretserver: error creating the random root key. err: %s\n", err)
		return nil, err
	}
	rootPObj := new(protected_objects.ProtectedObjectMessage)
	rootId := protected_objects.ObjectIdMessage{
		ObjName:  &rootName,
		ObjEpoch: &epoch,
	}
	rootPObj.ProtectedObjId = &rootId
	encrypted, err := encKey.CryptingKey.Encrypt(rootKey)
	if err != nil {
		log.Printf("secretserver: error encrypting the root key. err: %s\n", err)
		return nil, err
	}
	rootPObj.Blob = encrypted
	return rootPObj, nil
}

// Copyright (c) 2014, Google Inc. All rights reserved.
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

package tpm

import (
	"bytes"
	"fmt"
	"github.com/golang/protobuf/proto"
	"testing"
)

// Test GetRandom
func TestEndian(t *testing.T) {
	l := uint16(0xff12)
	v := byte(l >> 8)
	var s [2]byte
	s[0] = v
	v = byte(l & 0xff)
	s[1] = v
	fmt.Printf("Endian test: %x\n", s)
}

// Test GetRandom
func TestGetRandom(t *testing.T) {
	fmt.Printf("TestGetRandom\n")

	// Open TPM
	rw, err := OpenTPM("/dev/tpm0")
	if err != nil {
		fmt.Printf("OpenTPM failed %s\n", err)
		return
	}

	rand, err :=  GetRandom(rw, 16)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		t.Fatal("GetRandom failed\n")
	}
	fmt.Printf("rand: %x\n", rand)
	rw.Close()
}

// TestReadPcr tests a ReadPcr command.
func TestReadPcrs(t *testing.T) {
	fmt.Printf("TestReadPcrs\n")

	// Open TPM
	rw, err := OpenTPM("/dev/tpm0")
	if err != nil {
		fmt.Printf("OpenTPM failed %s\n", err)
		return
	}

	pcr := []byte{0x03, 0x80, 0x00, 0x00}
	counter, pcr_out, alg, digest, err := ReadPcrs(rw, byte(4), pcr)
	if err != nil {
		t.Fatal("ReadPcrs failed\n")
	}
	fmt.Printf("Counter: %x, pcr: %x, alg: %x, digest: %x\n", counter, pcr_out, alg, digest)
	rw.Close()
}

// TestReadClock tests a ReadClock command.
func TestReadClock(t *testing.T) {
	fmt.Printf("TestReadClock\n")

	// Open TPM
	rw, err := OpenTPM("/dev/tpm0")
	if err != nil {
		fmt.Printf("OpenTPM failed %s\n", err)
		return
	}
	current_time, current_clock, err := ReadClock(rw)
	if err != nil {
		t.Fatal("ReadClock failed\n")
	}
	fmt.Printf("current_time: %x , current_clock: %x\n", current_time, current_clock)
	rw.Close()

}

// TestGetCapabilities tests a GetCapabilities command.
// Command: 8001000000160000017a000000018000000000000014
func TestGetCapabilities(t *testing.T) {

	// Open TPM
	rw, err := OpenTPM("/dev/tpm0")
	if err != nil {
		fmt.Printf("OpenTPM failed %s\n", err)
		return
	}
	handles, err := GetCapabilities(rw, ordTPM_CAP_HANDLES, 1, 0x80000000)
	if err != nil {
		t.Fatal("GetCapabilities failed\n")
	}
	fmt.Printf("Open handles:\n")
	for _, e := range handles {
		fmt.Printf("    %x\n", e)
	}
	rw.Close()
}

// Combined Key Test
func TestCombinedKeyTest(t *testing.T) {

	// Open tpm
	rw, err := OpenTPM("/dev/tpm0")
	if err != nil {
		fmt.Printf("OpenTPM failed %s\n", err)
		return
	}

	// Flushall
	err =  Flushall(rw)
	if err != nil {
		t.Fatal("Flushall failed\n")
	}
	fmt.Printf("Flushall succeeded\n")

	// CreatePrimary
	var empty []byte
	primaryparms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00030072), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), uint16(0),
		uint16(1024), uint32(0x00010001), empty}
	parent_handle, public_blob, err := CreatePrimary(rw,
		uint32(ordTPM_RH_OWNER), []int{0x7}, "", "01020304", primaryparms)
	if err != nil {
		t.Fatal("CreatePrimary fails")
	}
	fmt.Printf("CreatePrimary succeeded\n")

	// CreateKey
	keyparms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00030072), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), uint16(0),
		uint16(1024), uint32(0x00010001), empty}
	private_blob, public_blob, err := CreateKey(rw, uint32(parent_handle),
		[]int{7}, "01020304", "01020304", keyparms)
	if err != nil {
		t.Fatal("CreateKey fails")
	}
	fmt.Printf("CreateKey succeeded, handle: %x\n", uint32(parent_handle))
	fmt.Printf("Private blob: %x\n", private_blob)
	fmt.Printf("Public  blob: %x\n\n", public_blob)

	// Load
	key_handle, blob, err := Load(rw, parent_handle, "", "01020304",
	     public_blob, private_blob)
	if err != nil {
		t.Fatal("Load fails")
	}
	fmt.Printf("Load succeeded, handle: %x\n", uint32(key_handle))
	fmt.Printf("Blob from Load     : %x\n", blob)

	// ReadPublic
	public, name, qualified_name, err := ReadPublic(rw, key_handle)
	if err != nil {
		t.Fatal("ReadPublic fails")
	}
	fmt.Printf("ReadPublic succeeded\n")
	fmt.Printf("Public	 blob: %x\n", public)
	fmt.Printf("Name	   blob: %x\n", name)
	fmt.Printf("Qualified name blob: %x\n\n", qualified_name)

	// Flush
	err = FlushContext(rw, key_handle)
	err = FlushContext(rw, parent_handle)
	rw.Close()
}

// Combined Seal test
func TestCombinedSealTest(t *testing.T) {

	// Open tpm
	rw, err := OpenTPM("/dev/tpm0")
	if err != nil {
		fmt.Printf("OpenTPM failed %s\n", err)
		return
	}

	// Flushall
	err =  Flushall(rw)
	if err != nil {
		t.Fatal("Flushall failed\n")
	}
	fmt.Printf("Flushall succeeded\n")

	// CreatePrimary
	var empty []byte
	primaryparms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00030072), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), uint16(0),
		uint16(1024), uint32(0x00010001), empty}
	parent_handle, public_blob, err := CreatePrimary(rw,
		uint32(ordTPM_RH_OWNER), []int{0x7}, "", "01020304", primaryparms)
	if err != nil {
		t.Fatal("CreatePrimary fails")
	}
	fmt.Printf("CreatePrimary succeeded\n")

	nonceCaller := []byte{0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}
	var secret []byte
	sym := uint16(algTPM_ALG_NULL)
	to_seal := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			  0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	hash_alg := uint16(algTPM_ALG_SHA1)

	session_handle, policy_digest, err := StartAuthSession(rw,
		Handle(ordTPM_RH_NULL),
		Handle(ordTPM_RH_NULL), nonceCaller, secret,
		uint8(ordTPM_SE_POLICY), sym, hash_alg)
	if err != nil {
		FlushContext(rw, parent_handle)
		t.Fatal("StartAuthSession fails")
	}
	fmt.Printf("StartAuth succeeds, handle: %x\n", uint32(session_handle))
	fmt.Printf("policy digest  : %x\n", policy_digest)

	err = PolicyPassword(rw, session_handle)
	if err != nil {
		FlushContext(rw, parent_handle)
		FlushContext(rw, session_handle)
		t.Fatal("PolicyPcr fails")
	}
	var tpm_digest []byte
	err = PolicyPcr(rw, session_handle, tpm_digest, []int{7})
	if err != nil {
		FlushContext(rw, parent_handle)
		FlushContext(rw, session_handle)
		t.Fatal("PolicyPcr fails")
	}

	policy_digest, err = PolicyGetDigest(rw, session_handle)
	if err != nil {
		FlushContext(rw, parent_handle)
		FlushContext(rw, session_handle)
		t.Fatal("PolicyGetDigest after PolicyPcr fails")
	}
	fmt.Printf("policy digest after PolicyPcr: %x\n", policy_digest)

	// CreateSealed
	keyedhashparms := KeyedHashParams{uint16(algTPM_ALG_KEYEDHASH),
		uint16(algTPM_ALG_SHA1),
		uint32(0x00000012), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), empty}
	private_blob, public_blob, err := CreateSealed(rw, parent_handle, policy_digest,
		"01020304",  "01020304", to_seal, []int{7}, keyedhashparms)
	if err != nil {
		FlushContext(rw, parent_handle)
		FlushContext(rw, session_handle)
		t.Fatal("CreateSealed fails")
	}

	// Load
	item_handle, blob, err := Load(rw, parent_handle, "", "01020304",
		public_blob, private_blob)
	if err != nil {
		FlushContext(rw, session_handle)
		FlushContext(rw, item_handle)
		FlushContext(rw, parent_handle)
		t.Fatal("Load fails")
	}
	fmt.Printf("Load succeeded, handle: %x\n", uint32(item_handle))
	fmt.Printf("Blob from Load     : %x\n\n", blob)

	// Unseal
	unsealed, nonce, err := Unseal(rw, item_handle, "01020304",
		session_handle, policy_digest)
	if err != nil {
		FlushContext(rw, item_handle)
		FlushContext(rw, parent_handle)
		t.Fatal("Unseal fails")
	}
	fmt.Printf("Unseal succeeds\n")
	fmt.Printf("unsealed           : %x\n", unsealed)
	fmt.Printf("nonce              : %x\n\n", nonce)

	// Flush
	FlushContext(rw, item_handle)
	FlushContext(rw, parent_handle)
	FlushContext(rw, session_handle)
	rw.Close()
	if bytes.Compare(to_seal, unsealed) != 0 {
		t.Fatal("seal and unsealed bytes dont match")
	}
}

// Combined Quote test
func TestCombinedQuoteTest(t *testing.T) {

	// Open tpm
	rw, err := OpenTPM("/dev/tpm0")
	if err != nil {
		fmt.Printf("OpenTPM failed %s\n", err)
		return
	}

	// Flushall
	err =  Flushall(rw)
	if err != nil {
		t.Fatal("Flushall failed\n")
	}
	fmt.Printf("Flushall succeeded\n\n")

	// CreatePrimary
	var empty []byte
	primaryparms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00030072), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), uint16(0),
		uint16(1024), uint32(0x00010001), empty}
	parent_handle, public_blob, err := CreatePrimary(rw,
		uint32(ordTPM_RH_OWNER), []int{0x7}, "", "01020304", primaryparms)
	if err != nil {
		t.Fatal("CreatePrimary fails")
	}
	fmt.Printf("CreatePrimary succeeded\n\n")

	// Pcr event
	eventData := []byte{1,2,3}
	err =  PcrEvent(rw, 7, eventData)
	if err != nil {
		t.Fatal("PcrEvent fails")
	}

	// CreateKey (Quote Key)
	keyparms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00050072), empty, uint16(algTPM_ALG_NULL), uint16(0),
		uint16(algTPM_ALG_ECB), uint16(algTPM_ALG_RSASSA),
		uint16(algTPM_ALG_SHA1),
		uint16(1024), uint32(0x00010001), empty}

	private_blob, public_blob, err := CreateKey(rw, uint32(parent_handle),
		[]int{7}, "01020304", "01020304", keyparms)
	if err != nil {
		t.Fatal("CreateKey fails")
	}
	fmt.Printf("CreateKey succeeded\n")
	fmt.Printf("Private blob: %x\n", private_blob)
	fmt.Printf("Public  blob: %x\n", public_blob)

	// Load
	quote_handle, blob, err := Load(rw, parent_handle, "", "01020304",
	     public_blob, private_blob)
	if err != nil {
		t.Fatal("Load fails")
	}
	fmt.Printf("Load succeeded, handle: %x\n", uint32(quote_handle))
	fmt.Printf("Blob from Load        : %x\n\n", blob)

	// Quote
	to_quote := []byte{0x0f,0x0e,0x0d,0x0c,0x0b,0x0a,0x09,0x08,
			   0x07,0x06,0x05,0x04,0x03,0x02,0x01,0x00}
	attest, sig, err := Quote(rw, quote_handle, "01020304", "01020304",
		to_quote, []int{7}, uint16(algTPM_ALG_NULL))
	if err != nil {
		FlushContext(rw, quote_handle)
		rw.Close()
		t.Fatal("Quote fails")
	}
	fmt.Printf("attest             : %x\n", attest)
	fmt.Printf("sig                : %x\n\n", sig)

	// get info for verify
	keyblob, name, qualified_name, err := ReadPublic(rw, quote_handle)
	if err != nil {
		FlushContext(rw, quote_handle)
		err = FlushContext(rw, parent_handle)
		rw.Close()
		t.Fatal("Quote fails")
	}

	// Flush
	err = FlushContext(rw, quote_handle)
	err = FlushContext(rw, parent_handle)
	rw.Close()

	// Verify quote
	fmt.Printf("keyblob(%x): %x\n", len(keyblob), keyblob)
	fmt.Printf("name(%x): %x\n", len(name), name)
	fmt.Printf("qualified_name(%x): %x\n", len(qualified_name), qualified_name)
	rsaParams, err := DecodeRsaBuf(public_blob)
	if err != nil {
		t.Fatal("DecodeRsaBuf fails %s", err)
	}
	PrintRsaParams(rsaParams)

	var quote_key_info QuoteKeyInfoMessage 
	att := int32(rsaParams.attributes)
	quote_key_info.Name = name
	quote_key_info.Properties = &att
	quote_key_info.PublicKey = new(PublicKeyMessage)
	key_type := "rsa"
	quote_key_info.PublicKey.KeyType = &key_type
	quote_key_info.PublicKey.RsaKey = new(RsaPublicKeyMessage)
	key_name :=  "QuoteKey"
	quote_key_info.PublicKey.RsaKey.KeyName = &key_name
	sz_mod := int32(rsaParams.mod_sz)
	quote_key_info.PublicKey.RsaKey.BitModulusSize = &sz_mod
	quote_key_info.PublicKey.RsaKey.Exponent = []byte{0,1,0,1}
	quote_key_info.PublicKey.RsaKey.Modulus =  rsaParams.modulus
	if !VerifyQuote(to_quote, quote_key_info, uint16(algTPM_ALG_SHA1), attest, sig) {
		t.Fatal("VerifyQuote fails")
	}
	fmt.Printf("VerifyQuote succeeds\n")
}

// Combined Endorsement/Activate test
func TestCombinedEndorsementTest(t *testing.T) {
	hash_alg_id := uint16(algTPM_ALG_SHA1)

	// Open tpm
	rw, err := OpenTPM("/dev/tpm0")
	if err != nil {
		fmt.Printf("OpenTPM failed %s\n", err)
		return
	}

	// Flushall
	err =  Flushall(rw)
	if err != nil {
		t.Fatal("Flushall failed\n")
	}
	fmt.Printf("Flushall succeeded\n\n")

	// CreatePrimary
	var empty []byte
	primaryparms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00030072), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), uint16(0),
		uint16(2048), uint32(0x00010001), empty}
	parent_handle, public_blob, err := CreatePrimary(rw,
		uint32(ordTPM_RH_OWNER), []int{0x7}, "", "", primaryparms)
	if err != nil {
		t.Fatal("CreatePrimary fails")
	}
	fmt.Printf("CreatePrimary succeeded\n\n")

	// CreateKey
	keyparms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00030072), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), uint16(0),
		uint16(2048), uint32(0x00010001), empty}
	private_blob, public_blob, err := CreateKey(rw, uint32(parent_handle),
		[]int{7}, "", "01020304", keyparms)
	if err != nil {
		t.Fatal("CreateKey fails")
	}
	fmt.Printf("CreateKey succeeded\n")
	fmt.Printf("Private blob: %x\n", private_blob)
	fmt.Printf("Public  blob: %x\n\n", public_blob)

	// Load
	key_handle, blob, err := Load(rw, parent_handle, "", "",
	     public_blob, private_blob)
	if err != nil {
		t.Fatal("Load fails")
	}
	fmt.Printf("Load succeeded\n")
	fmt.Printf("\nBlob from Load     : %x\n", blob)

	// ReadPublic
	public, name, _, err := ReadPublic(rw, key_handle)
	if err != nil {
		t.Fatal("ReadPublic fails")
	}
	fmt.Printf("ReadPublic succeeded\n")
	fmt.Printf("Public         blob: %x\n", public)

	// Generate Credential
	credential := []byte{1,2,3,4,5,6,7,8,9,0xa,0xb,0xc,0xd,0xe,0xf,0x10}
	fmt.Printf("Credential: %x\n", credential)

	// Internal MakeCredential
	credBlob, encrypted_secret0, err := InternalMakeCredential(rw, parent_handle, credential, name)
	if err != nil {
		FlushContext(rw, key_handle)
		FlushContext(rw, parent_handle)
		t.Fatal("Can't InternalMakeCredential\n")
	}
	fmt.Printf("\nencrypted secret   : %x\n", encrypted_secret0)
	fmt.Printf("name                 : %x\n", name)
	fmt.Printf("credBlob             : %x\n", credBlob)

	// ActivateCredential
	recovered_credential1, err := ActivateCredential(rw, key_handle, parent_handle,
		"01020304", "", credBlob, encrypted_secret0)
	if err != nil {
		FlushContext(rw, key_handle)
		FlushContext(rw, parent_handle)
		t.Fatal("Can't ActivateCredential\n")
	}
	fmt.Printf("Restored Credential, test 1: %x\n", recovered_credential1)
	if bytes.Compare(credential, recovered_credential1) != 0 {
		FlushContext(rw, key_handle)
		FlushContext(rw, parent_handle)
		t.Fatal("Credential and recovered credential differ\n")
	}
	fmt.Printf("Make/Activate test 1 succeeds\n")

	// Get endorsement cert
	der_endorsement_cert := RetrieveFile("/home/jlm/cryptobin/endorsement_cert")
	if der_endorsement_cert == nil {
		FlushContext(rw, key_handle)
		FlushContext(rw, parent_handle)
		t.Fatal("Can't retrieve endorsement cert\n")
	}

	// MakeCredential
	encrypted_secret, encIdentity, integrityHmac, err := MakeCredential(
		der_endorsement_cert, hash_alg_id, credential, name)
	if err != nil {
		FlushContext(rw, key_handle)
		FlushContext(rw, parent_handle)
		t.Fatal("Can't MakeCredential\n")
	}
	fmt.Printf("\nencrypted secret   : %x\n", encrypted_secret)
	fmt.Printf("encIdentity        : %x\n", encIdentity)
	fmt.Printf("integrityHmac      : %x\n\n", integrityHmac)

	// ActivateCredential
	recovered_credential2, err := ActivateCredential(rw,
		key_handle, parent_handle, "01020304", "",
		append(integrityHmac, encIdentity...), encrypted_secret)
	if err != nil {
		FlushContext(rw, key_handle)
		FlushContext(rw, parent_handle)
		t.Fatal("Can't ActivateCredential\n")
	}
	fmt.Printf("Restored Credential, test 2: %x\n", recovered_credential2)
	if bytes.Compare(credential, recovered_credential2) != 0 {
		FlushContext(rw, key_handle)
		FlushContext(rw, parent_handle)
		t.Fatal("Credential and recovered credential differ\n")
	}
	fmt.Printf("Make/Activate test 2 succeeds\n")

	// Flush
	FlushContext(rw, key_handle)
	FlushContext(rw, parent_handle)
	rw.Close()
}

// Combined Evict test
func TestCombinedEvictTest(t *testing.T) {
	fmt.Printf("TestCombinedEvictTest excluded\n")
	return

	// Open tpm
	rw, err := OpenTPM("/dev/tpm0")
	if err != nil {
		fmt.Printf("OpenTPM failed %s\n", err)
		return
	}

	// Flushall
	err =  Flushall(rw)
	if err != nil {
		t.Fatal("Flushall failed\n")
	}
	fmt.Printf("Flushall succeeded\n")

	// CreatePrimary
	var empty []byte
	primaryparms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00030072), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), uint16(0),
		uint16(1024), uint32(0x00010001), empty}
	parent_handle, public_blob, err := CreatePrimary(rw,
		uint32(ordTPM_RH_OWNER), []int{0x7}, "", "01020304", primaryparms)
	if err != nil {
		t.Fatal("CreatePrimary fails")
	}
	fmt.Printf("CreatePrimary succeeded\n")

	// CreateKey
	keyparms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00030072), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), uint16(0),
		uint16(1024), uint32(0x00010001), empty}
	private_blob, public_blob, err := CreateKey(rw, uint32(parent_handle),
		[]int{7}, "01020304", "01020304", keyparms)
	if err != nil {
		t.Fatal("CreateKey fails")
	}
	fmt.Printf("CreateKey succeeded\n")
	fmt.Printf("Private blob: %x\n", private_blob)
	fmt.Printf("Public  blob: %x\n\n", public_blob)

	// Load
	key_handle, blob, err := Load(rw, parent_handle, "", "01020304",
	     public_blob, private_blob)
	if err != nil {
		t.Fatal("Load fails")
	}
	fmt.Printf("Load succeeded\n")
	fmt.Printf("\nBlob from Load     : %x\n", blob)

	// ReadPublic
	public, name, qualified_name, err := ReadPublic(rw, key_handle)
	if err != nil {
		t.Fatal("ReadPublic fails")
	}
	fmt.Printf("ReadPublic succeeded\n")
	fmt.Printf("Public         blob: %x\n", public)
	fmt.Printf("Name           blob: %x\n", name)
	fmt.Printf("Qualified name blob: %x\n\n", qualified_name)

	perm_handle := uint32(0x810003e8)

	// Evict
	err = EvictControl(rw, Handle(ordTPM_RH_OWNER), key_handle, "", "01020304",
		Handle(perm_handle))
	if err != nil {
		t.Fatal("EvictControl 1 fails")
	}

	// Evict
	err = EvictControl(rw, Handle(ordTPM_RH_OWNER), Handle(perm_handle), "", "01020304",
		Handle(perm_handle))
	if err != nil {
		t.Fatal("EvictControl 1 fails")
	}

	// Flush
	err = FlushContext(rw, key_handle)
	err = FlushContext(rw, parent_handle)
	rw.Close()
}

// Combined Context test
func TestCombinedContextTest(t *testing.T) {
	fmt.Printf("TestCombinedContextTest excluded\n")
	return
	// pcr selections
	// CreatePrimary
	// SaveContext
	// FlushContext
	// LoadContext
	// FlushContext
}

// Combined Quote Protocol
func TestCombinedQuoteProtocolTest(t *testing.T) {
	fmt.Printf("TestCombinedQuoteProtocolTest excluded\n")
	return

	// Read der-encoded private policy key
	der_policy_key := RetrieveFile("/home/jlm/cryptobin/cloudproxy_key_file")
	if der_policy_key == nil {
		t.Fatal("Can't open private key file")
	}

	// Read der-encoded policy cert
	der_policy_cert := RetrieveFile("/home/jlm/cryptobin/policy_cert")
	if der_policy_cert == nil {
		t.Fatal("Can't open private key file")
	}

	// Read endorsement cert file
	der_endorsement_cert := RetrieveFile("/home/jlm/cryptobin/endorsement_cert")
	if der_endorsement_cert == nil {
		t.Fatal("Can't open private key file")
	}
	fmt.Printf("Got endorsement cert: %x\n\n", der_endorsement_cert)

	// Open tpm
	rw, err := OpenTPM("/dev/tpm0")
	if err != nil {
		t.Fatal("Can't open tpm")
	}

	// Open endorsement and quote keys
	var empty []byte
	ek_parms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00030072), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), uint16(0),
		uint16(1024), uint32(0x00010001), empty}
	endorsement_handle, _, err := CreatePrimary(rw, uint32(ordTPM_RH_OWNER), []int{7},
		"", "01020304", ek_parms)
	if err != nil {
		t.Fatal("CreatePrimary fails")
	}
	quote_parms := RsaParams{uint16(algTPM_ALG_RSA), uint16(algTPM_ALG_SHA1),
		uint32(0x00030072), empty, uint16(algTPM_ALG_AES), uint16(128),
		uint16(algTPM_ALG_CFB), uint16(algTPM_ALG_NULL), uint16(0),
		uint16(1024), uint32(0x00010001), empty}
	private_blob, public_blob, err := CreateKey(rw, uint32(ordTPM_RH_OWNER), []int{7},
						    "", "01020304", quote_parms)
	if err != nil {
		t.Fatal("Create fails")
	}
	fmt.Printf("Create Key for quote succeeded\n")
	fmt.Printf("Private: %x\n", private_blob)
	fmt.Printf("Public: %x\n\n", public_blob)

	quote_handle, quote_blob, err := Load(rw, endorsement_handle, "", "01020304",
		public_blob, private_blob)
	if err != nil {
		t.Fatal("Quote Load fails")
	}
	fmt.Printf("Load succeeded, blob size: %d\n\n", len(quote_blob))

	der_program_private, request_message, err := ConstructClientRequest(rw, der_endorsement_cert,
		quote_handle, "", "01020304", "Test-Program-1")
	if err != nil {
		t.Fatal("ConstructClientRequest fails")
	}
	fmt.Printf("der_program_private size: %d\n", len(der_program_private))
	fmt.Printf("Request: %s\n", proto.MarshalTextString(request_message))

	signing_instructions_message := new(SigningInstructionsMessage)
	response_message, err := ConstructServerResponse(der_policy_cert,
		der_policy_key, *signing_instructions_message, *request_message)
	if err != nil {
		t.Fatal("ConstructServerResponse fails")
	}

	der_program_cert, err := ClientDecodeServerResponse(rw, endorsement_handle,
		quote_handle, "01020304", *response_message)
	if err != nil {
		t.Fatal("ClientDecodeServerResponse fails")
	}

	// Save Program cert
	fmt.Printf("Program cert: %x\n", der_program_cert)

	// Close handles
	FlushContext(rw, endorsement_handle)
	FlushContext(rw, quote_handle)
}

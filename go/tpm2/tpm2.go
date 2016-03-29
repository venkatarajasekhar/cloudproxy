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

// Package tpm supports direct communication with a tpm 2.0 device under Linux.
package tpm2

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/golang/protobuf/proto"
)

// OpenTPM opens a channel to the TPM at the given path. If the file is a
// device, then it treats it like a normal TPM device, and if the file is a
// Unix domain socket, then it opens a connection to the socket.
func OpenTPM(path string) (io.ReadWriteCloser, error) {
	// If it's a regular file, then open it
	var rwc io.ReadWriteCloser
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if fi.Mode()&os.ModeDevice != 0 {
		var f *os.File
		f, err = os.OpenFile(path, os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		rwc = io.ReadWriteCloser(f)
	} else if fi.Mode()&os.ModeSocket != 0 {
		uc, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
		if err != nil {
			return nil, err
		}
		rwc = io.ReadWriteCloser(uc)
	} else {
		return nil, fmt.Errorf("unsupported TPM file mode %s", fi.Mode().String())
	}

	return rwc, nil
}

func PrintAttestData(parms *Attest) {
	fmt.Printf("Magic_number: %x\n", parms.Magic_number)
	fmt.Printf("Attest_type : %x\n", parms.Attest_type)
	fmt.Printf("Name        : %x\n", parms.Name)
	fmt.Printf("Data        : %x\n", parms.Data)
	fmt.Printf("Clock       : %x\n", parms.Clock)
	fmt.Printf("ResetCount  : %x\n", parms.ResetCount)
	fmt.Printf("RestartCount: %x\n", parms.RestartCount)
	fmt.Printf("Safe        : %x\n", parms.Safe)
	fmt.Printf("FirmwareVersion: %x\n", parms.FirmwareVersion)
	fmt.Printf("PcrSelect   : %x\n", parms.PcrSelect)
	fmt.Printf("PcrDigest   : %x\n", parms.PcrDigest)
}

func PrintKeyedHashParams(parms *KeyedHashParams) {
	fmt.Printf("Type_alg   : %x\n", parms.Type_alg)
	fmt.Printf("Hash_alg   : %x\n", parms.Hash_alg)
	fmt.Printf("Attributes : %x\n", parms.Attributes)
	fmt.Printf("Auth_policy: %x\n", parms.Auth_policy)
	fmt.Printf("Symalg     : %x\n", parms.Symalg)
	fmt.Printf("Sym_sz     : %x\n", parms.Sym_sz)
	fmt.Printf("Mode       : %x\n", parms.Mode)
	fmt.Printf("Scheme     : %x\n", parms.Scheme)
	fmt.Printf("Unique     : %x\n", parms.Unique)
}

func PrintRsaParams(parms *RsaParams) {
	fmt.Printf("Enc_alg     : %x\n", parms.Enc_alg)
	fmt.Printf("Hash_alg    : %x\n", parms.Hash_alg)
	fmt.Printf("Attributes  : %x\n", parms.Attributes)
	fmt.Printf("Auth_policy : %x\n", parms.Auth_policy)
	fmt.Printf("Symalg      : %x\n", parms.Symalg)
	fmt.Printf("Sym_sz      : %x\n", parms.Sym_sz)
	fmt.Printf("Mode        : %x\n", parms.Mode)
	fmt.Printf("Scheme      : %x\n", parms.Scheme)
	fmt.Printf("Scheme_hash : %x\n", parms.Scheme_hash)
	fmt.Printf("Modulus size: %x\n", parms.Mod_sz)
	fmt.Printf("Exp         : %x\n", parms.Exp)
	fmt.Printf("Modulus     : %x\n", parms.Modulus)
}

func SetShortPcrs(pcr_nums []int) ([]byte, error) {
	pcr := []byte{3,0,0,0}
	var byte_num int
	var byte_pos byte
	for _,e := range pcr_nums {
		byte_num = 1 + e / 8;
		byte_pos = 1 << uint16(e % 8)
		pcr[byte_num] |= byte_pos
	}
	return pcr, nil
}

// nil is error
func SetHandle(handle Handle) ([]byte) {
	uint32_handle := uint32(handle)
	str,_ := pack([]interface{}{&uint32_handle})
	return str
}

// nil return is an error
func SetPasswordData(password string) ([]byte) {
// len password
	pw, err := hex.DecodeString(password)
	if err != nil {
		return nil
	}
	ret, _ := pack([]interface{}{&pw})
	return ret
}

// nil return is an error
// 	returns: len0 TPM_RS_PW 0000 01 password data as []byte
func CreatePasswordAuthArea(password string, owner Handle) ([]byte) {
	owner_str := SetHandle(owner)
	suffix := []byte{0, 0, 1}
	pw := SetPasswordData(password)
	final_buf := append(owner_str, suffix...)
	final_buf = append(final_buf, pw...)
	out := []interface{}{&final_buf}
	ret, _:= pack(out)
	return ret
}

// nil is error
func CreateSensitiveArea(in1 []byte, in2 []byte) ([]byte) {
//   password (SENSITIVE CREATE)
//   0008 0004 01020304
//        0000
	t1, err := pack([]interface{}{&in1})
	if err != nil {
		return nil
	}
	t2, err := pack([]interface{}{&in2})
	if err != nil {
		return nil
	}

	t := append(t1, t2...)
	ret, err := pack([]interface{}{&t})
	if err != nil {
		return nil
	}

	return ret
}

func DecodeRsaBuf(rsa_buf []byte) (*RsaParams, error) {
	parms := new(RsaParams)
	current := int(0)
	template := []interface{}{&parms.Enc_alg, &parms.Hash_alg,
			&parms.Attributes, &parms.Auth_policy}
	err := unpack(rsa_buf[current:], template)
	if err != nil {
		return nil, errors.New("Can't unpack Rsa buffer 2")
	}
	current += 10 + len(parms.Auth_policy)
	template = []interface{}{&parms.Symalg}
	err = unpack(rsa_buf[current:], template)
	if err != nil {
		return nil, errors.New("Can't unpack Rsa buffer 3")
	}
	current += 2
	if parms.Symalg != uint16(AlgTPM_ALG_NULL) {
		template = []interface{}{&parms.Sym_sz, &parms.Mode}
		err = unpack(rsa_buf[current:], template)
		if err != nil {
			return nil, errors.New("Can't unpack Rsa buffer 4")
		}
		current += 4
	} else {
		parms.Sym_sz = 0
		parms.Mode = 0
		parms.Scheme = 0
	}
	template = []interface{}{&parms.Scheme}
	err = unpack(rsa_buf[current:], template)
	if err != nil {
		return nil, errors.New("Can't unpack Rsa buffer 5")
	}
	current += 2
	if parms.Scheme == uint16(AlgTPM_ALG_RSASSA) {
		template = []interface{}{&parms.Scheme_hash}
		err = unpack(rsa_buf[current:], template)
		if err != nil {
			return nil, errors.New("Can't unpack Rsa buffer 6")
		}
		current += 2
	}

	template = []interface{}{&parms.Mod_sz, &parms.Exp, &parms.Modulus}
	err = unpack(rsa_buf[current:], template)
	if err != nil {
		return nil, errors.New("Can't unpack Rsa buffer 7")
	}
	return parms, nil
}

// nil is error
func DecodeRsaArea(in []byte) (*RsaParams, error) {
	var rsa_buf []byte

	template := []interface{}{&rsa_buf}
	err := unpack(in, template)
	if err != nil {
		return nil, errors.New("Can't unpack Rsa buffer 1")
	}
	return DecodeRsaBuf(rsa_buf)
}

// nil is error
func CreateKeyedHashParams(parms KeyedHashParams) ([]byte) {
	// 0 (uint16), type, attributes, auth, scheme, 0 (unique)
	template := []interface{}{&parms.Type_alg, &parms.Hash_alg,
			&parms.Attributes, &parms.Auth_policy, &parms.Scheme,
			&parms.Unique}
	t1, err := pack(template)
	if err != nil {
		return nil
	}
	return t1
}

// nil return is error
func CreateRsaParams(parms RsaParams) ([]byte) {
	template := []interface{}{&parms.Enc_alg, &parms.Hash_alg,
				  &parms.Attributes, &parms.Auth_policy}
	t1, err := pack(template)
	if err != nil {
		return nil
	}

	if  parms.Symalg !=  uint16(AlgTPM_ALG_NULL) {
		template = []interface{}{&parms.Symalg, &parms.Sym_sz,
				   &parms.Mode, &parms.Scheme}
	} else {
		template = []interface{}{&parms.Symalg, &parms.Scheme}
	}
	t2, err := pack(template)
	if err != nil {
		return nil
	}
	if parms.Scheme == uint16(AlgTPM_ALG_RSASSA) {
		template3 := []interface{}{&parms.Scheme_hash}
		t3, err := pack(template3)
		if err != nil {
			return nil
		}
		t2 = append(t2, t3...)
	}

	template4 := []interface{}{&parms.Mod_sz, &parms.Exp, parms.Modulus}
	t4, err := pack(template4)
	if err != nil {
		return nil
	}

	t5 := append(t1, t2...)
	t5 = append(t5, t4...)
	template5 := []interface{}{&t5}
	buf, err := pack(template5)
	if err != nil {
		return nil
	}
	return buf
}

// nil return is error
func CreateLongPcr(count uint32, pcr_nums []int) ([]byte) {
	if count == 0 {
		b1, err := pack([]interface{}{&count})
		if err != nil {
			return nil
		}
		return b1
	}
	b1, err :=  SetShortPcrs(pcr_nums)
	if err != nil {
		return nil
	}
	template := []interface{}{&count, &b1}
	b2, err := pack(template)
	if err != nil {
		return nil
	}
	return b2
}

// Retieve file.
func RetrieveFile(fileName string) ([]byte) {
	fileInfo, err := os.Stat(fileName)
	if err != nil {
		return nil
	}
	buf := make([]byte, fileInfo.Size())
	fileHandle, err := os.Open(fileName)
	if err != nil {
		return nil
	}
	read, err := fileHandle.Read(buf)
	if int64(read) < fileInfo.Size() || err != nil {
		fileHandle.Close()
	return nil
	}
	fileHandle.Close()
	return buf
}

// ConstructGetRandom constructs a GetRandom command.
func ConstructGetRandom(size uint32) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdGetRandom)
	if err != nil {
		return nil, errors.New("ConstructGetRandom failed")
	}
	num_bytes :=  []interface{}{uint16(size)}
	cmd, _ := packWithHeader(cmdHdr, num_bytes)
	return cmd, nil
}

// DecodeGetRandom decodes a GetRandom response.
func DecodeGetRandom(in []byte) ([]byte, error) {
var rand_bytes []byte

	out :=  []interface{}{&rand_bytes}
	err := unpack(in, out)
	if err != nil {
		return nil, errors.New("Can't decode GetRandom response")
	}
	return rand_bytes, nil
}

// GetRandom gets random bytes from the TPM.
func GetRandom(rw io.ReadWriteCloser, size uint32) ([]byte, error) {
	// Construct command
	cmd, err:= ConstructGetRandom(size)
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return nil, err
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, errors.New("Write Tpm fails") 
	}

	// Get response
	resp := make([]byte, 1024, 1024)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return nil, errors.New("Read buffer too small")
	}
	_, size, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse ", err, "\n")
		return nil, err
	}
	if status != ErrSuccess {
	}
	rand, err :=  DecodeGetRandom(resp[10:read])
	if err != nil {
		fmt.Printf("DecodeGetRandom ", err, "\n")
		return nil,err
	}
	return rand, nil
}

// ConstructFlushContext constructs a FlushContext command.
func ConstructFlushContext(handle Handle) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdFlushContext)
	if err != nil {
		return nil, errors.New("ConstructFlushContext failed")
	}
	cmd_text :=  []interface{}{uint32(handle)}
	x, _ := packWithHeader(cmdHdr, cmd_text)
	return x, nil
}

// FlushContext
func FlushContext(rw io.ReadWriter, handle Handle) (error) {
	// Construct command
	cmd, err:= ConstructFlushContext(handle)
	if err != nil {
		return errors.New("ConstructFlushContext fails") 
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 1024, 1024)
	read, err := rw.Read(resp)
	if err != nil {
		return errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return errors.New("DecodeCommandResponse fails")
	}
	if status != ErrSuccess {
		return errors.New("FlushContext unsuccessful")
	}
	return nil
}

// ConstructReadPcrs constructs a ReadPcr command.
func ConstructReadPcrs(num_spec int, num_pcr byte, pcrs []byte) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdPCR_Read)
	if err != nil {
		return nil, errors.New("ConstructReadPcrs failed")
	}
	num := uint32(num_spec)
	template := []interface{}{&num, &pcrs}
	cmd, _ := packWithHeader(cmdHdr, template)
	return cmd, nil
}

// DecodeReadPcrs decodes a ReadPcr response.
func DecodeReadPcrs(in []byte) (uint32, []byte, uint16, []byte, error) {
	var pcr []byte
	var digest []byte
	var updateCounter uint32
	var t uint32
	var s uint32

	out :=  []interface{}{&t, &updateCounter, &pcr, &s, &digest}
	err := unpack(in, out)
	if err != nil {
		return 1, nil, 0, nil, errors.New("Can't decode ReadPcrs response")
	}
	return updateCounter, pcr, uint16(t), digest, nil
}

// ReadPcr reads a PCR value from the TPM.
//	Output: updatecounter, selectout, digest
func ReadPcrs(rw io.ReadWriter, num_byte byte, pcrSelect []byte) (uint32, []byte, uint16, []byte, error) {
	// Construct command
	x, err:= ConstructReadPcrs(1, 4, pcrSelect)
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return 1, nil, 0, nil, errors.New("MakeCommandHeader failed") 
	}

	// Send command
	_, err = rw.Write(x)
	if err != nil {
		return 0, nil, 0, nil, errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 1024, 1024)
	read, err := rw.Read(resp)
	if err != nil {
		return 0, nil, 0, nil, errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return 0, nil, 0, nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return 0, nil, 0, nil, errors.New("DecodeCommandResponse fails")
	}
	if status != ErrSuccess {
		return 0, nil, 0, nil, errors.New("ReadPcr command failed")
	}
	counter, pcr, alg, digest, err := DecodeReadPcrs(resp[10:])
	if err != nil {
		return 0, nil, 0, nil, errors.New("DecodeReadPcrsfails")
	}
	return counter, pcr, alg, digest, err 
}

// ConstructReadClock constructs a ReadClock command.
func ConstructReadClock() ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdReadClock)
	if err != nil {
		return nil, errors.New("ConstructGetRandom failed")
	}
	cmd := packWithBytes(cmdHdr, nil)
	return cmd, nil
}

// DecodeReadClock decodes a ReadClock response.
func DecodeReadClock(in []byte) (uint64, uint64, error) {
	var current_time, current_clock uint64

	template :=  []interface{}{&current_time, &current_clock}
	err := unpack(in, template)
	if err != nil {
		return 0, 0, errors.New("Can't decode DecodeReadClock response")
	}
	return current_time, current_clock, nil
}

// ReadClock
//	Output: current time, current clock
func ReadClock(rw io.ReadWriter) (uint64, uint64, error) {
	// Construct command
	x, err:= ConstructReadClock()
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return 0 ,0, err
	}

	// Send command
	_, err = rw.Write(x)
	if err != nil {
		return 0, 0, errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 1024, 1024)
	read, err := rw.Read(resp)
	if err != nil {
		return 0, 0, errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return 0, 0, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse %s\n", err)
		return 0, 0, err
	}
	if status != ErrSuccess {
	}
	current_time, current_clock, err :=  DecodeReadClock(resp[10:read])
	if err != nil {
		fmt.Printf("DecodeReadClock %s\n", err)
		return 0, 0,err
	}
	return current_time, current_clock, nil
}

// ConstructGetCapabilities constructs a GetCapabilities command.
func ConstructGetCapabilities(cap uint32, count uint32, property uint32) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdGetCapability)
	if err != nil {
		return nil, errors.New("GetCapability failed")
	}
	cap_bytes:=  []interface{}{&cap, &property, &count}
	cmd, _ := packWithHeader(cmdHdr, cap_bytes)
	return cmd, nil
}

// DecodeGetCapabilities decodes a GetCapabilities response.
func DecodeGetCapabilities(in []byte) (uint32, []uint32, error) {
	var num_handles uint32
	var cap_reported uint32

	out :=  []interface{}{&cap_reported, &num_handles}
	err := unpack(in[1:9], out)
	if err != nil {
		return 0, nil, errors.New("Can't decode GetCapabilities response")
	}
	// only OrdTPM_CAP_HANDLES handled
	if cap_reported !=  OrdTPM_CAP_HANDLES {
		return 0, nil, errors.New("Only ordTPM_CAP_HANDLES supported")
	}
	var handles []uint32
	var handle uint32
	handle_out :=  []interface{}{&handle}
	for i:= 0; i < int(num_handles); i++ {
		err := unpack(in[8 + 4 * i:12 + 4 * i], handle_out)
		if err != nil {
			return 0, nil, errors.New("Can't decode GetCapabilities handle")
		}
		handles = append(handles, handle)
	}

	return cap_reported, handles, nil
}

// GetCapabilities 
//	Output: output buf
func GetCapabilities(rw io.ReadWriter, cap uint32, count uint32, property uint32) ([]uint32, error) {
	// Construct command
	cmd, err:= ConstructGetCapabilities(cap, count, property)
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return nil, err
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, errors.New("Write Tpm fails") 
	}

	// Get response
	resp := make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse %s\n", err)
		return nil, err
	}
	if status != ErrSuccess {
	}
	_, handles, err :=  DecodeGetCapabilities(resp[10:read])
	if err != nil {
		return nil,err
	}
	return handles, nil
}

// ConstructPcrEvent
func ConstructPcrEvent(pcrnum int, eventData []byte) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdPcrEvent)
	if err != nil {
		return nil, errors.New("GetCapability failed")
	}
	// pcrnum, empty, emptyauth, eventData size, eventData
	var empty []byte
	pc := uint32(pcrnum)
	b1,_ := pack([]interface{}{&pc, &empty})
	b2 := CreatePasswordAuthArea("", Handle(OrdTPM_RS_PW))
	b3,_ := pack([]interface{}{&eventData})
	cmd := packWithBytes(cmdHdr, append(append(b1, b2...),b3...))
	return cmd, nil
}

// PcrEvent
func PcrEvent(rw io.ReadWriter, pcrnum int, eventData []byte) (error) {
	// Construct command
	cmd, err:= ConstructPcrEvent(pcrnum, eventData)
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return err
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse %s\n", err)
		return err
	}
	if status != ErrSuccess {
		return errors.New("Command failure")
	}
	return nil
}

// Flushall
func Flushall(rw io.ReadWriter) (error) {
	handles, err := GetCapabilities(rw, OrdTPM_CAP_HANDLES, 1, 0x80000000)
	if err != nil {
		return err
	}
	for _, e := range handles {
		_ = FlushContext(rw, Handle(e))
	}
	return nil
}

// ConstructCreatePrimary constructs a CreatePrimary command.
func ConstructCreatePrimary(owner uint32, pcr_nums []int,
		parent_password string, owner_password string,
		parms RsaParams) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdCreatePrimary)
	if err != nil {
		return nil, errors.New("ConstructCreatePrimary failed")
	}
	var empty []byte
	b1 := SetHandle(Handle(owner))
	b2,_ := pack([]interface{}{&empty})
	b3 := CreatePasswordAuthArea(parent_password, Handle(OrdTPM_RS_PW))
	t1 := SetPasswordData(owner_password)
	b4 := CreateSensitiveArea(t1[2:], empty)
	b5 := CreateRsaParams(parms)
	b6,_ := pack([]interface{}{&empty})
	var b7 []byte
	if len(pcr_nums) > 0 {
		b7 = CreateLongPcr(uint32(1), pcr_nums)
	} else {
		b7 = CreateLongPcr(uint32(0), pcr_nums)
	}
	arg_bytes := append(b1, b2...)
	arg_bytes = append(arg_bytes, b3...)
	arg_bytes = append(arg_bytes, b4...)
	arg_bytes = append(arg_bytes, b5...)
	arg_bytes = append(arg_bytes, b6...)
	arg_bytes = append(arg_bytes, b7...)
	cmd_bytes := packWithBytes(cmdHdr, arg_bytes)
	return cmd_bytes, nil
}

// DecodeCreatePrimary decodes a CreatePrimary response.
func DecodeCreatePrimary(in []byte) (Handle, []byte, error) {
	var handle uint32
	var auth []byte

	// handle and auth data
	template :=  []interface{}{&handle, &auth}
	err := unpack(in, template)
	if err != nil {
		return Handle(0), nil, errors.New("Can't decode response 1")
	}

	var current int
	current = 6 + 2*len(auth)
	// size, size-public
	var tpm2_public []byte
	template =  []interface{}{&tpm2_public}
	err = unpack(in[current:], template)
	if err != nil {
		return Handle(0), nil, errors.New("Can't decode CreatePrimary response 2")
	}

	var rsa_params_buf []byte
	template =  []interface{}{&rsa_params_buf}
	err = unpack(tpm2_public, template)
	if err != nil {
		return Handle(0), nil, errors.New("Can't decode CreatePrimary response 3")
	}

	// params
	params, err := DecodeRsaArea(tpm2_public)
	if err != nil {
		return Handle(0), nil, err
	}
	PrintRsaParams(params)

	// Creation data
	current = 2 + len(rsa_params_buf)
	var creation_data []byte
	template =  []interface{}{&creation_data}
	err = unpack(tpm2_public[current:], template)
	if err != nil {
		return Handle(0), nil, errors.New("Can't decode CreatePrimary response 4")
	}
	current += len(creation_data) +2

	// Digest
	var digest []byte
	template =  []interface{}{&digest}
	err = unpack(tpm2_public[current:], template)
	if err != nil {
		return Handle(0), nil, errors.New("Can't decode CreatePrimary response 5")
	}
	current += len(digest) +2

	// TPMT_TK_CREATION
	current += 6
	var crap []byte
	template =  []interface{}{&crap}
	err = unpack(tpm2_public[current:], template)
	if err != nil {
		return Handle(0), nil, errors.New("Can't decode CreatePrimary response 5")
	}
	current += len(crap) +2

	// Name
	var name []byte
	template =  []interface{}{&name}
	err = unpack(tpm2_public[current:], template)
	if err != nil {
		return Handle(0), nil, errors.New("Can't decode CreatePrimary response 5")
	}

	return Handle(handle), tpm2_public, nil
}

// CreatePrimary
//	Output: handle, public key blob
func CreatePrimary(rw io.ReadWriter, owner uint32, pcr_nums []int,
	parent_password, owner_password string, parms RsaParams) (Handle, []byte, error) {

	// Construct command
	cmd, err:= ConstructCreatePrimary(uint32(owner), pcr_nums, parent_password,
		owner_password, parms)
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return Handle(0), nil, err
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return Handle(0), nil, errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 2048, 2048)
	read, err := rw.Read(resp)
	if err != nil {
		return Handle(0), nil, errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return Handle(0), nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse %s\n", err)
		return Handle(0), nil, err
	}
	if status != ErrSuccess {
	}
	handle, public_blob, err :=  DecodeCreatePrimary(resp[10:read])
	if err != nil {
		fmt.Printf("DecodeCreatePrimary %s\n", err)
		return Handle(0), nil, err
	}
	return Handle(handle), public_blob, nil
}

// ConstructReadPublic constructs a ReadPublic command.
func ConstructReadPublic(handle Handle) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdReadPublic)
	if err != nil {
		return nil, errors.New("ConstructReadPublic failed")
	}
	num_bytes :=  []interface{}{uint32(handle)}
	cmd, _ := packWithHeader(cmdHdr, num_bytes)
	return cmd, nil
}

// DecodeReadPublic decodes a ReadPublic response.
//	public, name, qualified name
func DecodeReadPublic(in []byte) ([]byte, []byte, []byte, error) {
	var public_blob []byte
	var name []byte
	var qualified_name []byte

	out :=  []interface{}{&public_blob, &name, &qualified_name}
	err := unpack(in, out)
	if err != nil {
		return nil, nil, nil, errors.New("Can't decode ReadPublic response")
	}
	return public_blob, name, qualified_name, nil
}

// ReadPublic
//	Output: key blob, name, qualified name
func ReadPublic(rw io.ReadWriter, handle Handle) ([]byte, []byte, []byte, error) {

	// Construct command
	cmd, err:= ConstructReadPublic(handle)
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return nil, nil, nil, err
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, nil, nil, errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, nil, nil, errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return nil, nil, nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse %s\n", err)
		return nil, nil, nil, err
	}
	if status != ErrSuccess {
		return nil, nil, nil, err
	}
	public_blob, name, qualified_name, err :=  DecodeReadPublic(resp[10:read])
	if err != nil {
		fmt.Printf("DecodeReadPublic %s\n", err)
		return nil, nil, nil,err
	}
	return public_blob, name, qualified_name, nil
}

// CreateKey

// ConstructCreateKey constructs a CreateKey command.
func ConstructCreateKey(owner uint32, pcr_nums []int, parent_password string, owner_password string,
		parms RsaParams) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdCreate)
	if err != nil {
		return nil, errors.New("ConstructCreateKey failed")
	}
	var empty []byte
	b1 := SetHandle(Handle(owner))
	b2 ,_ := pack([]interface{}{&empty})
	b3 := CreatePasswordAuthArea(parent_password, Handle(OrdTPM_RS_PW))
	t1 := SetPasswordData(owner_password)
	b4 := CreateSensitiveArea(t1[2:], empty)
	b5 := CreateRsaParams(parms)
	b6 ,_ := pack([]interface{}{&empty})
	b7:= CreateLongPcr(uint32(1), pcr_nums)
	arg_bytes := append(b1, b2...)
	arg_bytes = append(arg_bytes, b3...)
	arg_bytes = append(arg_bytes, b4...)
	arg_bytes = append(arg_bytes, b5...)
	arg_bytes = append(arg_bytes, b6...)
	arg_bytes = append(arg_bytes, b7...)
	cmd_bytes := packWithBytes(cmdHdr, arg_bytes)
	return cmd_bytes, nil
}

// DecodeCreateKey decodes a CreateKey response.
//	Output: private_blob, public_blob
func DecodeCreateKey(in []byte) ([]byte, []byte, error) {
	var tpm2b_private []byte
	var tpm2b_public []byte

	// auth?
	// tpm2b_private
	// tpm2b_public
	out :=  []interface{}{&tpm2b_private, &tpm2b_public}
	err := unpack(in[4:], out)
	if err != nil {
		return nil, nil, errors.New("Can't decode CreateKey response")
	}
	// creation data
	// tpmt_tk_creation
	// digest
	return tpm2b_private, tpm2b_public, nil
}

// Output: public blob, private blob, digest
func CreateKey(rw io.ReadWriter, owner uint32, pcr_nums []int, parent_password string, owner_password string,
		parms RsaParams) ([]byte, []byte, error) {

	// Construct command
	cmd, err:= ConstructCreateKey(uint32(owner), pcr_nums, parent_password, owner_password, parms)
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return nil, nil, err
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, nil, errors.New("Write Tpm fails") 
	}
	fmt.Printf("CreateKey cmd : %x\n", cmd)

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, nil, errors.New("Read Tpm fails")
	}
	fmt.Printf("CreateKey resp: %x\n", resp[0:read])

	// Decode Response
	if read < 10 {
		return nil, nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse %s\n", err)
		return nil, nil, err
	}
	if status != ErrSuccess {
		return nil, nil, errors.New("Error from command")
	}
	private_blob, public_blob, err :=  DecodeCreateKey(resp[10:read])
	if err != nil {
		fmt.Printf("DecodeCreateKey %s\n", err)
		return nil, nil, err
	}
	return private_blob, public_blob, nil
}

// ConstructLoad constructs a Load command.
func ConstructLoad(parentHandle Handle, parentAuth string, ownerAuth string,
	     public_blob []byte, private_blob []byte) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdLoad)
	if err != nil {
		return nil, errors.New("ConstructLoad failed")
	}
	b1 := SetHandle(parentHandle)
	b3 := SetPasswordData(parentAuth)
	b4 := CreatePasswordAuthArea(ownerAuth, Handle(OrdTPM_RS_PW))
	// private, public
	b5,_ := pack([]interface{}{&private_blob, &public_blob})
	arg_bytes := append(b1, b3...)
	arg_bytes = append(arg_bytes, b4...)
	arg_bytes = append(arg_bytes, b5...)
	cmd_bytes := packWithBytes(cmdHdr, arg_bytes)
	return cmd_bytes, nil
}

// DecodeLoad decodes a Load response.
//	handle, name
func DecodeLoad(in []byte) (Handle, []byte, error) {
	var handle uint32
	var auth []byte
	var name []byte

	out :=  []interface{}{&handle, &auth, &name}
	err := unpack(in, out)
	if err != nil {
		return Handle(0), nil, errors.New("Can't decode Load response")
	}
	return Handle(handle), name, nil
}

// Load
//	Output: handle
func Load(rw io.ReadWriter, parentHandle Handle, parentAuth string, ownerAuth string,
	     public_blob []byte, private_blob []byte) (Handle, []byte, error) {

	// Construct command
	cmd, err:= ConstructLoad(parentHandle, parentAuth, ownerAuth, public_blob, private_blob)
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return Handle(0), nil, err
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return Handle(0), nil, errors.New("Write Tpm fails") 
	}
	fmt.Printf("Load cmd: %x\n", cmd)

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return Handle(0), nil, errors.New("Read Tpm fails")
	}
	fmt.Printf("Load resp: %x\n", resp[0:read])

	// Decode Response
	if read < 10 {
		return Handle(0), nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse %s\n", err)
		return Handle(0), nil, err
	}
	if status != ErrSuccess {
		return Handle(0), nil, errors.New("Error from command")
	}
	handle, name, err :=  DecodeLoad(resp[10:read])
	if err != nil {
		fmt.Printf("DecodeCreateKey %s\n", err)
		return Handle(0), nil, err
	}
	return handle, name, nil
}

// Construct PolicyPcr command.
func ConstructPolicyPcr(handle Handle, expected_digest []byte, pcr_nums []int) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdPolicyPCR)
	if err != nil {
		return nil, errors.New("ConstructPcr failed")
	}
	u_handle := uint32(handle)
	template :=  []interface{}{&u_handle, &expected_digest}
	b1, err := pack(template)
	if err != nil {
		return nil, errors.New("Can't pack pcr buf")
	}
	b2 := CreateLongPcr(1, pcr_nums)
	cmd := packWithBytes(cmdHdr, append(b1, b2...))
	return cmd, nil
}

// ConstructPolicyPassword constructs a PolicyPassword command.
func ConstructPolicyPassword(handle Handle) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdPolicyPassword)
	if err != nil {
		return nil, errors.New("ConstructPassword failed")
	}
	u_handle := uint32(handle)
	template :=  []interface{}{&u_handle}
	b1, err := pack(template)
	if err != nil {
		return nil, errors.New("Can't pack pcr buf")
	}
	cmd := packWithBytes(cmdHdr, b1)
	return cmd, nil
}

// PolicyPassword
func PolicyPassword(rw io.ReadWriter, handle Handle) (error) {
	// Construct command
	cmd, err:= ConstructPolicyPassword(handle)
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return err
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 1024, 1024)
	read, err := rw.Read(resp)
	if err != nil {
		return errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse %s\n", err)
		return err
	}
	if status != ErrSuccess {
		return errors.New("Comand failure")
	}
	return nil
}

// PolicyPcr
func PolicyPcr(rw io.ReadWriter, handle Handle, expected_digest []byte, pcr_nums []int) (error) {
	// Construct command
	cmd, err:= ConstructPolicyPcr(handle, expected_digest, pcr_nums)
	if err != nil {
		fmt.Printf("MakeCommandHeader failed %s\n", err)
		return err
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 1024, 1024)
	read, err := rw.Read(resp)
	if err != nil {
		return errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse %s\n", err)
		return err
	}
	if status != ErrSuccess {
		return errors.New("Comand failure")
	}
	return nil
}


// ConstructPolicyGetDigest constructs a PolicyGetDigest command.
func ConstructPolicyGetDigest(handle Handle) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdPolicyGetDigest)
	if err != nil {
		return nil, errors.New("ConstructGetDigest failed")
	}
	u_handle := uint32(handle)
	template :=  []interface{}{&u_handle}
	b1, err := pack(template)
	if err != nil {
		return nil, errors.New("Can't pack pcr buf")
	}
	cmd := packWithBytes(cmdHdr, b1)
	return cmd, nil
}

// DecodePolicyGetDigest decodes a PolicyGetDigest response.
func DecodePolicyGetDigest(in []byte) ([]byte, error) {
	var digest []byte

	out :=  []interface{}{&digest}
	err := unpack(in, out)
	if err != nil {
		return nil, errors.New("Can't decode DecodePolicyGetDigest response")
	}
	return digest, nil
}

// PolicyGetDigest
//	Output: digest
func PolicyGetDigest(rw io.ReadWriter, handle Handle) ([]byte, error) {
	// Construct command
	cmd, err:= ConstructPolicyGetDigest(handle)
	if err != nil {
		fmt.Printf("ConstructPolicyGetDigest failed %s\n", err)
		return nil, err
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		fmt.Printf("DecodeCommandResponse %s\n", err)
		return nil, err
	}
	if status != ErrSuccess {
		return nil, errors.New("Comand failure")
	}
	digest, err := DecodePolicyGetDigest(resp[10:])
	if err != nil {
		return nil, err
	}
	return digest, nil
}

// ConstructStartAuthSession constructs a StartAuthSession command.
func ConstructStartAuthSession(tpm_key Handle, bind_key Handle,
		nonceCaller []byte, secret []byte,
		se byte, sym uint16, hash_alg uint16) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdStartAuthSession)
	if err != nil {
		return nil, errors.New("ConstructStartAuthSession failed")
	}
	b1 := SetHandle(tpm_key)
	b2 := SetHandle(bind_key)
	b3 ,_ := pack([]interface{}{&nonceCaller, &secret})
	// secret and se
	b4 := []byte{se}
	b5 ,_ := pack([]interface{}{&sym, &hash_alg})
	arg_bytes := append(b1, b2...)
	arg_bytes = append(arg_bytes, b3...)
	arg_bytes = append(arg_bytes, b4...)
	arg_bytes = append(arg_bytes, b5...)
	cmd_bytes := packWithBytes(cmdHdr, arg_bytes)
	return cmd_bytes, nil
}

// DecodeStartAuthSession decodes a StartAuthSession response.
//	Output: session_handle, nonce
func DecodeStartAuthSession(in []byte) (Handle, []byte, error) {
	var handle uint32
	var nonce []byte
	template :=  []interface{}{&handle, &nonce}
	err := unpack(in, template)
	if err != nil {
		return Handle(0), nil, errors.New("Can't decode StartAuthSession response")
	}
	return Handle(handle), nonce, nil
}

// StartAuthSession
func StartAuthSession(rw io.ReadWriter, tpm_key Handle, bind_key Handle,
		nonceCaller []byte, secret []byte,
		se byte, sym uint16, hash_alg uint16) (Handle, []byte, error) {

	// Construct command
	cmd, err:= ConstructStartAuthSession(tpm_key, bind_key, nonceCaller, secret,
		se, sym, hash_alg)
	if err != nil {
		return Handle(0), nil, errors.New("ConstructStartAuthSession fails")
	}
	fmt.Printf("StartAuth cmd: %d\n", cmd)

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return Handle(0), nil, errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return Handle(0), nil, errors.New("Read Tpm fails")
	}
	fmt.Printf("StartAuth resp: %d\n", resp[0:read])

	// Decode Response
	if read < 10 {
		return Handle(0), nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return Handle(0), nil, errors.New("DecodeCommandResponse fails")
	}
	if status != ErrSuccess {
		return Handle(0), nil, errors.New("StartAuthSession unsuccessful")
	}
	handle, nonce, err := DecodeStartAuthSession(resp[10:])
	if err != nil {
		return Handle(0), nil, errors.New("DecodeStartAuthSession fails")
	}
	return handle, nonce, nil
}

// ConstructCreateSealed constructs a CreateSealed command.
func ConstructCreateSealed(parent Handle, policy_digest []byte,
			   parent_password string, owner_password string,
			   to_seal []byte, pcr_nums []int,
			   parms KeyedHashParams) ([]byte, error) {
	PrintKeyedHashParams(&parms)
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdCreate)
	if err != nil {
		return nil, errors.New("ConstructCreateKey failed")
	}
	var empty []byte
	b1 := SetHandle(parent)
	b2 ,_ := pack([]interface{}{&empty})
	b3 := CreatePasswordAuthArea(parent_password, Handle(OrdTPM_RS_PW))
	t1 := SetPasswordData(owner_password)
	b4 := CreateSensitiveArea(t1[2:], to_seal)
	parms.Auth_policy =  policy_digest
	b5 := CreateKeyedHashParams(parms)
	b6 ,_ := pack([]interface{}{&b5})
	b7, _ := pack([]interface{}{&empty})
	b8:= CreateLongPcr(uint32(1), pcr_nums)
	arg_bytes := append(b1, b2...)
	arg_bytes = append(arg_bytes, b3...)
	arg_bytes = append(arg_bytes, b4...)
	arg_bytes = append(arg_bytes, b6...)
	arg_bytes = append(arg_bytes, b7...)
	arg_bytes = append(arg_bytes, b8...)
	cmd_bytes := packWithBytes(cmdHdr, arg_bytes)
	return cmd_bytes, nil
}

// DecodeCreateSealed decodes a CreateSealed response.
// 	Output: private, public, creation_out, digest_out, creation_ticket
func DecodeCreateSealed(in []byte) ([]byte, []byte, error) {
	var tpm2b_private []byte
	var tpm2b_public []byte

	// auth, tpm2b_private, tpm2b_public
	template :=  []interface{}{&tpm2b_private, &tpm2b_public}
	err := unpack(in[4:], template)
	if err != nil {
		return nil, nil, errors.New("Can't decode CreateSealed response")
	}
	// creation data
	// tpmt_tk_creation
	// digest
	return tpm2b_private, tpm2b_public, nil
}

// CreateSealed
// 	Output: public blob, private blob
func CreateSealed(rw io.ReadWriter, parent Handle, policy_digest []byte,
		  parent_password string, owner_password string,
		  to_seal []byte, pcr_nums []int, parms KeyedHashParams) ([]byte, []byte, error) {
	// Construct command
	cmd, err:= ConstructCreateSealed(parent, policy_digest,
			parent_password, owner_password,
			to_seal, pcr_nums, parms)
	if err != nil {
		return nil, nil, errors.New("ConstructCreateSealed fails") 
	}
	fmt.Printf("Seal cmd: %x\n", cmd)

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, nil, errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, nil, errors.New("Read Tpm fails")
	}
	fmt.Printf("Seal resp: %x\n", resp[0:read])

	// Decode Response
	if read < 10 {
		return nil, nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return nil, nil, errors.New("DecodeCommandResponse fails")
	}
	if status != ErrSuccess {
		return nil, nil, errors.New("CreateSealed unsuccessful")
	}
	handle, nonce, err := DecodeCreateSealed(resp[10:])
	if err != nil {
		return nil, nil, errors.New("DecodeCreateSealed fails")
	}
	return handle, nonce, nil
}

// ConstructUnseal constructs a Unseal command.
func ConstructUnseal(item_handle Handle, password string, session_handle Handle) ([]byte, error)  {
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdUnseal)
	if err != nil {
		return nil, errors.New("Construct Unseal failed")
	}
	// item_handle
	var empty []byte
	handle1 := uint32(item_handle)
	template :=  []interface{}{&handle1, &empty}
	b1, err := pack(template)
	if err != nil {
		return nil, errors.New("Can't construct Unseal")
	}
	session_attributes := uint8(1)
	b2 := CreatePasswordAuthArea(password, session_handle)
	template =  []interface{}{&empty, &session_attributes}  // null hmac
	cmd_bytes := packWithBytes(cmdHdr, append(b1, b2...))
	return cmd_bytes, nil
}

// DecodeUnseal decodes a Unseal response.
//	Output: sensitive data
func DecodeUnseal(in []byte) ([]byte, []byte, error) {
	var unsealed []byte
	var digest []byte

	template :=  []interface{}{&unsealed, &digest}
	err := unpack(in[4:], template)
	if err != nil {
		return nil, nil, errors.New("Can't decode Unseal response")
	}
	return unsealed, digest, nil
}

// Unseal
func Unseal(rw io.ReadWriter, item_handle Handle, password string, session_handle Handle,
		digest []byte) ([]byte, []byte, error) {
	// Construct command
	cmd, err:= ConstructUnseal(item_handle, password, session_handle)
	if err != nil {
		return nil, nil, errors.New("ConstructUnseal fails") 
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, nil, errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, nil, errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return nil, nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return nil, nil, errors.New("DecodeCommandResponse fails")
	}
	if status != ErrSuccess {
		return nil, nil, errors.New("Unseal unsuccessful")
	}
	unsealed, nonce, err := DecodeUnseal(resp[10:])
	if err != nil {
		return nil, nil, errors.New("DecodeStartAuthSession fails")
	}
	return unsealed, nonce, nil
}

// ConstructQuote constructs a Quote command.
func ConstructQuote(signing_handle Handle, parent_password, owner_password string,
	to_quote []byte, pcr_nums []int, sig_alg uint16) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdQuote)
	if err != nil {
		return nil, errors.New("ConstructQuote failed")
	}
	// TODO: no scheme or sig_alg
	// handle
	var empty []byte
	b1 := SetHandle(signing_handle)
	b2 ,_ := pack([]interface{}{&empty})
	b3 := CreatePasswordAuthArea(parent_password, Handle(OrdTPM_RS_PW))
	b4 ,_ := pack([]interface{}{&to_quote, &sig_alg})
	b5 := CreateLongPcr(uint32(1), pcr_nums)
	arg_bytes := append(b1, b2...)
	arg_bytes = append(arg_bytes, b3...)
	arg_bytes = append(arg_bytes, b4...)
	// Scheme info?
	arg_bytes = append(arg_bytes, b5...)
	// arg_bytes = append(arg_bytes, b6...)
	cmd_bytes := packWithBytes(cmdHdr, arg_bytes)
	return cmd_bytes, nil
}

// DecodeQuote decodes a Quote response.
//	Output: attest, signature
func DecodeQuote(in []byte) ([]byte, uint16, uint16, []byte, error) {
	var empty []byte
	var buf []byte
	var attest []byte
	var signature []byte
	var s1 uint16
	var s2 uint16

	template :=  []interface{}{&empty, &buf}
	err := unpack(in, template)
	if err != nil {
		return nil, 0, 0, nil, errors.New("Can't decode Quote response")
	}

	template =  []interface{}{&attest, &s1, &s2, &signature}
	err = unpack(buf, template)
	if err != nil {
		return nil, 0, 0, nil, errors.New("Can't decode Quote response")
	}
	return attest, s1, s2, signature, nil
}

// Quote
// 	Output: attest, sig
func Quote(rw io.ReadWriter, signing_handle Handle, parent_password string, owner_password string,
		to_quote []byte, pcr_nums []int, sig_alg uint16) ([]byte, []byte, error) {
	// Construct command
	cmd, err:= ConstructQuote(signing_handle, parent_password, owner_password,
				  to_quote, pcr_nums, sig_alg)
	if err != nil {
		return nil, nil, errors.New("ConstructQuote fails") 
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, nil, errors.New("Write Tpm fails") 
	}
	fmt.Printf("\nQuote command: %x\n", cmd)

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, nil, errors.New("Read Tpm fails")
	}
	fmt.Printf("Quote resp: %x\n", resp[0:read])

	// Decode Response
	if read < 10 {
		return nil, nil, errors.New("Read buffer too small")
	}
	fmt.Printf("Quote resp: %x\n", resp[0:read])
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return nil, nil, errors.New("DecodeCommandResponse fails")
	}
	if status != ErrSuccess {
		return nil, nil, errors.New("Quote unsuccessful")
	}
	attest, _, _, sig, err := DecodeQuote(resp[10:])
	if err != nil {
		return nil, nil, errors.New("DecodeQuote fails")
	}
	return attest, sig, nil
}

// ConstructActivateCredential constructs a ActivateCredential command.
func ConstructActivateCredential(active_handle Handle, key_handle Handle,
		activePassword string, protectorPassword string,
		credBlob []byte, secret []byte) ([]byte, error) {
	var empty []byte
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdActivateCredential)
	if err != nil {
		return nil, errors.New("ConstructActivateCredential failed")
	}
	b1 := SetHandle(active_handle)
	b2 := SetHandle(key_handle)
	b3, _ := pack([]interface{}{&empty})
	b4a := CreatePasswordAuthArea(activePassword, Handle(OrdTPM_RS_PW))
	b4b := CreatePasswordAuthArea(protectorPassword, Handle(OrdTPM_RS_PW))
	b4t := append(b4a[2:], b4b[2:]...)
	b4, _ := pack([]interface{}{&b4t})
	b5, _ := pack([]interface{}{&credBlob, &secret})
	arg_bytes := append(b1, b2...)
	arg_bytes = append(arg_bytes, b3...)
	arg_bytes = append(arg_bytes, b4...)
	arg_bytes = append(arg_bytes, b5...)
	cmd_bytes := packWithBytes(cmdHdr, arg_bytes)
	return cmd_bytes, nil
}

// DecodeActivateCredential decodes a ActivateCredential response.
// returns certInfo
func DecodeActivateCredential(in []byte) ([]byte, error) {
	var empty []byte
	var buf []byte
	var certInfo []byte

	template :=  []interface{}{&empty, &buf}
	err := unpack(in, template)
	if err != nil {
		return nil, errors.New("Can't decode ActivateCredential response")
	}
	template =  []interface{}{&certInfo}
	err = unpack(buf, template)
	if err != nil {
		return nil, errors.New("Can't decode ActivateCredential response")
	}
	return certInfo, nil
}

// ActivateCredential
// 	Output: certinfo
func ActivateCredential(rw io.ReadWriter, active_handle Handle, key_handle Handle,
		activePassword string, protectorPassword string,
		credBlob []byte, secret []byte) ([]byte, error) {
	// Construct command
	cmd, err:= ConstructActivateCredential (active_handle, key_handle, activePassword,
		protectorPassword, credBlob, secret)
	if err != nil {
		return nil, errors.New("ConstructActivateCredential fails") 
	}
	fmt.Printf("ActivateCredential  cmd: %x\n", cmd)

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, errors.New("Read Tpm fails")
	}
	fmt.Printf("ActivateCredential resp: %x\n", resp[0:read])

	// Decode Response
	if read < 10 {
		return nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return nil, errors.New("DecodeCommandResponse fails")
	}
	if status != ErrSuccess {
		return nil, errors.New("ActivateCredential unsuccessful")
	}
	cred, err := DecodeActivateCredential(resp[10:])
	if err != nil {
		return nil, errors.New("DecodeActivateCredential fails")
	}
	return cred, nil
}

// ConstructEvictControl constructs a EvictControl command.
func ConstructEvictControl(owner Handle, tmp_handle Handle,
		persistant_handle Handle) ([]byte, error) {
	var empty []byte
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdEvictControl)
	if err != nil {
		return nil, errors.New("ConstructEvictControl failed")
	}
	b1 := SetHandle(owner)
	b2 := SetHandle(tmp_handle)
	b3, err := pack([]interface{}{&empty})
	if err != nil {
		return nil, errors.New("can't encode empty")
	}
	b4 := CreatePasswordAuthArea("", Handle(OrdTPM_RS_PW))
	b5 := SetHandle(persistant_handle)
	arg_bytes := append(b1, b2...)
	arg_bytes = append(arg_bytes, b3...)
	arg_bytes = append(arg_bytes, b4...)
	arg_bytes = append(arg_bytes, b5...)
	cmd_bytes := packWithBytes(cmdHdr, arg_bytes)
	return cmd_bytes, nil
}

// DecodeEvictControl decodes a EvictControl response.
func DecodeEvictControl(in []byte) (error) {
	return nil
}

// EvictControl
func EvictControl(rw io.ReadWriter, owner Handle, tmp_handle Handle, persistant_handle Handle) (error) {
	// Construct command
	cmd, err:= ConstructEvictControl(owner, tmp_handle, persistant_handle)
	if err != nil {
		return errors.New("ConstructEvictControl fails") 
	}
	fmt.Printf("Evict Control cmd : %x\n", cmd)

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 1024, 1024)
	read, err := rw.Read(resp)
	if err != nil {
		return errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return errors.New("DecodeCommandResponse fails")
	}
	if status != ErrSuccess {
		return errors.New("EvictControl unsuccessful")
	}
	err = DecodeEvictControl(resp[10:])
	if err != nil {
		return errors.New("DecodeEvictControl fails")
	}
	return nil
}

// ConstructSaveContext constructs a SaveContext command.
func ConstructSaveContext(handle Handle) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdContextSave)
	if err != nil {
		return nil, errors.New("ConstructSaveContext failed")
	}
	b1 := SetHandle(handle)
	cmd_bytes := packWithBytes(cmdHdr, b1)
	return cmd_bytes, nil
}

// DecodeSaveContext constructs a SaveContext command.
func DecodeSaveContext(save_area []byte) ([]byte, error) {
	return save_area, nil
}

func SaveContext(rw io.ReadWriter, handle Handle) ([]byte, error) {
	// Construct command
	cmd, err:= ConstructSaveContext(handle)
	if err != nil {
		return nil, errors.New("ConstructSaveContext fails") 
	}
	fmt.Printf("Save Context cmd : %x\n", cmd)

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 4096, 4096)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return nil, errors.New("Read buffer too small")
	}
	tag, size, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return nil, errors.New("DecodeCommandResponse fails")
	}
	fmt.Printf("SaveContext Tag: %x, size: %x, error code: %x\n", tag, size, status)
	if status != ErrSuccess {
		return nil, errors.New("SaveContext unsuccessful")
	}
	save_area, err := DecodeSaveContext(resp[10:])
	if err != nil {
		return nil, errors.New("DecodeSaveContext fails")
	}
	return save_area, nil
}

// LoadContext

// ConstructLoadContext constructs a LoadContext command.
func ConstructLoadContext(save_area []byte) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagSESSIONS, 0, cmdContextLoad)
 	if err != nil {
		return nil, errors.New("ConstructLoadContext failed")
	}
	cmd_bytes := packWithBytes(cmdHdr, save_area)
	return cmd_bytes, nil
}

// DecodeLoadContext decodes a LoadContext response.
func  DecodeLoadContext(in []byte) (Handle, error) {
	var handle uint32
	template :=  []interface{}{&handle}
	err := unpack(in, template)
	if err != nil {
		return Handle(0), errors.New("Can't decode LoadContext response")
	}
	return Handle(handle), nil
}

// LoadContext
func LoadContext(rw io.ReadWriter, save_area []byte) (Handle, error) {
	// Construct command
	cmd, err:= ConstructLoadContext(save_area)
	if err != nil {
		return Handle(0), errors.New("ConstructLoadContext fails") 
	}
	fmt.Printf("Load Context cmd : %x\n", cmd)

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return Handle(0), errors.New("Write Tpm fails") 
	}

	// Get response
	var resp []byte
	resp = make([]byte, 2048, 2048)
	read, err := rw.Read(resp)
	if err != nil {
		return Handle(0), errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return Handle(0), errors.New("Read buffer too small")
	}
	tag, size, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return Handle(0), errors.New("DecodeCommandResponse fails")
	}
	fmt.Printf("LoadContext Tag: %x, size: %x, error code: %x\n", tag, size, status)
	if status != ErrSuccess {
		return Handle(0), errors.New("LoadContext unsuccessful")
	}
	handle, err := DecodeLoadContext(resp[10:])
	if err != nil {
		return Handle(0), errors.New("DecodeLoadContext fails")
	}
	return handle, nil
}

func UnmarshalCertifyInfo(in []byte) (*Attest, error) {
	attest := new(Attest)
	var count uint32
	template := []interface{}{&attest.Magic_number, &attest.Attest_type, &attest.Name,
			&attest.Data, &attest.Clock, &attest.ResetCount,  &attest.RestartCount,
			&attest.Safe, &attest.FirmwareVersion, &count}
	err := unpack(in, template)
	if err != nil {
		return nil, err
	}
	i := 4+2+2+2+8+4+4+1+8+4+len(attest.Name)+len(attest.Data)
	attest.PcrSelect = in[i:i+4]
	template = []interface{}{&attest.PcrDigest}
	err = unpack(in[i+6:], template)
	if err != nil {
		return nil, err
	}
	return attest, nil
}

func ComputeHashValue(alg uint16, to_hash []byte) ([]byte, error) {
	if alg ==  uint16(AlgTPM_ALG_SHA1) {
		hash := sha1.New()
		hash.Write(to_hash)
		hash_value := hash.Sum(nil)
		return hash_value, nil
	} else if alg == uint16(AlgTPM_ALG_SHA256) {
		hash:= sha256.New()
		hash.Write(to_hash)
		hash_value := hash.Sum(nil)
		return hash_value, nil
	} else {
		return nil, errors.New("unsupported hash alg")
	}
}

func KDFA(alg uint16, key []byte, label string, contextU []byte, contextV []byte, bits int) ([]byte, error) {
	counter := uint32(0)
	bytes_left := (bits + 7) / 8;
	var out []byte
	for ; bytes_left > 0 ; {
		counter = counter + 1
		if alg == AlgTPM_ALG_SHA1 {
			mac := hmac.New(sha1.New, key)
			// copy counter (big Endian), label, contextU, contextV, bits (big Endian)
			outa,_ := pack([]interface{}{&counter})
			var arr [32]byte
			copy(arr[0:], label)
			arr[len(label)] = 0
			outc := append(contextU, contextV...)
			u_bits := uint32(bits)
			outd,_ := pack([]interface{}{&u_bits})
			in := append(outa, append(arr[0:len(label)+1], append(outc, outd...)...)...)
			mac.Write(in)
			out = append(out, mac.Sum(nil)...)
			bytes_left -= 20
		} else if alg == AlgTPM_ALG_SHA256 {
			mac := hmac.New(sha256.New, key)
			// copy counter (big Endian), label, contextU, contextV, bits (big Endian)
			outa, _ := pack([]interface{}{&counter})
			var arr [32]byte
			copy(arr[0:], label)
			arr[len(label)] = 0
			outc := append(contextU, contextV...)
			u_bits := uint32(bits)
			outd,_ := pack([]interface{}{&u_bits})
			in := append(outa, append(arr[0:len(label)+1], append(outc, outd...)...)...)
			mac.Write(in)
			out = append(out, mac.Sum(nil)...)
			bytes_left -= 32
		} else {
			return nil, errors.New("Unsupported key hmac alg")
		}
	}
	return out, nil
}

func ComputePcrDigest(alg uint16, in []byte) ([]byte, error) {
	// in should just be a sequence of digest values
	return ComputeHashValue(alg, in)
}

//	Return: out_hmac, output_data
func EncryptDataWithCredential(encrypt_flag bool, hash_alg_id uint16, unmarshaled_credential []byte,
		inData []byte, inHmac []byte) ([]byte, []byte, error) {
	var contextV []byte
	derivedKeys, err := KDFA(hash_alg_id, unmarshaled_credential, "PROTECT", contextV, contextV, 512)
	if err != nil {
		fmt.Printf("EncryptDataWithCredential can't derive keys\n")
		return nil, nil, errors.New("KDFA failed")
	}
	var calculatedHmac []byte
	outData := make([]byte, len(inData), len(inData))
	iv := derivedKeys[16:32]
	key := derivedKeys[0:16]
	dec, err := aes.NewCipher(key)
	ctr := cipher.NewCTR(dec, iv)
	ctr.XORKeyStream(outData, inData)

	var toHash []byte
	if encrypt_flag == true {
		toHash =  inData
	} else {
		toHash = outData
	}
	// Calculate hmac on output data
	if hash_alg_id == AlgTPM_ALG_SHA1 {
		hm := hmac.New(sha1.New, derivedKeys[48:64])
		hm.Write(toHash)
		calculatedHmac = hm.Sum(nil)
	} else if hash_alg_id == AlgTPM_ALG_SHA256 {
		hm := hmac.New(sha256.New, derivedKeys[32:64])
		hm.Write(toHash)
		calculatedHmac = hm.Sum(nil)
	} else {
		fmt.Printf("EncryptDataWithCredential unrecognized hmac alg\n")
		return nil, nil, errors.New("Unsupported Hash alg")
	}

	if encrypt_flag == false {
		if bytes.Compare(calculatedHmac, inHmac) != 0 {
			return nil, nil, errors.New("Integrity check fails")
		}
	}

	return calculatedHmac, outData, nil
}

// Returns encrypted secret.
func encryptHack (hash_alg_id uint16, modSize int,
		  protectorPublic *rsa.PublicKey, seed []byte,
		  label []byte) ([]byte, error) {

	private, err := rsa.GenerateKey(rand.Reader, modSize)
	if  err != nil || private == nil {
		return nil, errors.New("Can't gen private key")
	}
	public := &private.PublicKey

	var fake_encrypted_secret []byte
	if hash_alg_id == uint16(AlgTPM_ALG_SHA1) {
		fake_encrypted_secret, err = rsa.EncryptOAEP(sha1.New(),
			rand.Reader, public, seed, label)
	} else if hash_alg_id == uint16(AlgTPM_ALG_SHA256) {
		fake_encrypted_secret, err = rsa.EncryptOAEP(sha256.New(),
			rand.Reader, public, seed, label)
	} else {
		return nil, errors.New("Unsupported hash")
	}
	if  err != nil {
		return nil, errors.New("Can't fake encrypt")
	}
	fmt.Printf("encrypted_secret: %x\n", fake_encrypted_secret)
	var N *big.Int
	var D *big.Int
	var x *big.Int
	var z *big.Int
	N = public.N
	D = private.D
	x = new(big.Int)
	z = new(big.Int)
	x.SetBytes(fake_encrypted_secret)
	z = z.Exp(x, D, N)
	decrypted_pad := z.Bytes()
	fmt.Printf("decrypted with pad (%d): %x\n", len(decrypted_pad), decrypted_pad)
	// zero := []byte{0}
	// decrypted_pad = append(zero, decrypted_pad...)
	// fmt.Printf("new pad (%d): %x\n", len(decrypted_pad), decrypted_pad)

	// Now encrypt with real key
	var M *big.Int
	var E *big.Int
	var u *big.Int
	var w *big.Int
	M = protectorPublic.N
	E = big.NewInt(int64(protectorPublic.E))
	u = new(big.Int)
	w = new(big.Int)
	u.SetBytes(decrypted_pad)
	w = w.Exp(u, E, M)
	encrypted_secret  := w.Bytes()
	return encrypted_secret, nil
}

//	1. Generate Seed
//	2. encrypted_secret= E(protector_key, seed || "IDENTITY")
//	3. symKey ≔ KDFa (ekNameAlg, seed, “STORAGE”, name, NULL , bits)
//	4. encIdentity ≔ AesCFB(symKey, 0, credential)
//	5. HMACkey ≔ KDFa (ekNameAlg, seed, “INTEGRITY”, NULL, NULL, bits)
//	6. outerHMAC ≔ HMAC(HMACkey, encIdentity || Name)
//
//	Return (all []byte)
//		encrypted_secret
//		encIdentity
//		integrityHmac
func MakeCredential(protectorPublic *rsa.PublicKey, hash_alg_id uint16,
		unmarshaled_credential []byte,
		unmarshaled_name []byte) ([]byte, []byte, []byte, error) {
	var a [9]byte
	copy(a[0:9], "IDENTITY")

	// Seed.
	var seed [16]byte
	rand.Read(seed[0:16])

	// encrypt secret
	var encrypted_secret []byte
	var err error
	if hash_alg_id == uint16(AlgTPM_ALG_SHA1) {
		encrypted_secret, err = rsa.EncryptOAEP(sha1.New(),
			rand.Reader, protectorPublic, seed[0:16], a[0:9])
	} else if hash_alg_id == uint16(AlgTPM_ALG_SHA256) {
		encrypted_secret, err = rsa.EncryptOAEP(sha256.New(),
			rand.Reader, protectorPublic, seed[0:16], a[0:9])
	} else {
		return nil, nil, nil, errors.New("Unsupported hash")
	}
	if  err != nil {
		return nil, nil, nil, errors.New("Can't encrypt secret")
	}

	var symKey []byte
	iv := []byte{0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}
	if hash_alg_id == uint16(AlgTPM_ALG_SHA1) {
		symKey, err = KDFA(hash_alg_id, seed[0:16], "STORAGE",
			unmarshaled_name, nil, 128)
		if err !=nil {
			return nil, nil, nil, err
		}
	} else if hash_alg_id == uint16(AlgTPM_ALG_SHA256) {
		symKey, err = KDFA(hash_alg_id, seed[0:16], "STORAGE",
			unmarshaled_name, nil, 256)
		if err !=nil {
			return nil, nil, nil, err
		}
	} else {
		fmt.Printf("Unsupported hash %x\n",  hash_alg_id)
		return nil, nil, nil, errors.New("Unsupported hash alg") 
	}
	block, err := aes.NewCipher(symKey[0:16])
	if err !=nil {
		return nil, nil, nil, err
	}

	// encIdentity is encrypted(size || byte-stream), size in big endian
	marshaled_credential := make([]byte, 2 + len(unmarshaled_credential))
	encIdentity := make([]byte, 2 + len(unmarshaled_credential))
	l := uint16(len(unmarshaled_credential))
	marshaled_credential[0] = byte(l >> 8)
	marshaled_credential[1] = byte(l & 0xff)
	copy(marshaled_credential[2:], unmarshaled_credential)
	cfb := cipher.NewCFBEncrypter(block, iv)
	cfb.XORKeyStream(encIdentity, marshaled_credential)
	cfbdec := cipher.NewCFBDecrypter(block, iv)
	decrypted_credential := make([]byte, 2 + len(unmarshaled_credential))
	cfbdec.XORKeyStream(decrypted_credential, encIdentity)
	if bytes.Compare(marshaled_credential, decrypted_credential) != 0 {
		return nil, nil, nil,errors.New("decrypted cred mismatch") 
	}

	hmacKey, err := KDFA(hash_alg_id, seed[0:16], "INTEGRITY",
		nil, nil, 8*SizeHash(hash_alg_id))
	if err !=nil {
		return nil, nil, nil, err
	}

	var hmac_bytes []byte
	if hash_alg_id == uint16(AlgTPM_ALG_SHA1) {
		mac := hmac.New(sha1.New, hmacKey[0:20])
		mac.Write(append(encIdentity, unmarshaled_name...))
		hmac_bytes = mac.Sum(nil)
	} else if hash_alg_id == uint16(AlgTPM_ALG_SHA256) {
		mac := hmac.New(sha256.New, hmacKey[0:32])
		mac.Write(append(encIdentity, unmarshaled_name...))
		hmac_bytes = mac.Sum(nil)
	} else {
		return nil, nil, nil, errors.New("Unsupported has alg") 
	}
	marshalled_hmac, _ := pack([]interface{}{&hmac_bytes})
	return encrypted_secret, encIdentity, marshalled_hmac, nil
}

func publicKeyFromPrivate(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	default:
	return nil
	}
}

func GetSerialNumber() (*big.Int) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	sn, _ := rand.Int(rand.Reader, serialNumberLimit)
	return sn
}

func SizeHash(alg_id uint16) (int) {
	if alg_id == uint16(AlgTPM_ALG_SHA1) {
		return 20
	} else if alg_id == uint16(AlgTPM_ALG_SHA256) {
		return 32
	} else {
		return -1
	}
}

func ValidPcr(pcrSelect []byte, digest []byte) (bool) {
	return true
}

func VerifyDerCert(der_cert []byte, der_signing_cert []byte) (bool, error) {
	roots := x509.NewCertPool()
	opts := x509.VerifyOptions{
		Roots:   roots,
	}

	// Verify key
	policy_cert, err := x509.ParseCertificate(der_signing_cert)
	if err != nil {
		fmt.Printf("Signing ParseCertificate fails")
		return false, err
	}
	roots.AddCert(policy_cert)
	fmt.Printf("Root cert: %x\n", der_signing_cert)

	// Verify key
	cert, err := x509.ParseCertificate(der_cert)
	if err != nil {
		fmt.Printf("Cert ParseCertificate fails")
		return false, err
	}
	fmt.Printf("Cert: %x\n", cert)

	roots.AddCert(policy_cert)
	opts.Roots = roots
	chains, err := cert.Verify(opts)
	if err != nil {
		fmt.Printf("Verify fails ", err, "\n")
		return false, err
	}
	if chains != nil {
		fmt.Printf("Verify\n")
		return true, nil
	} else {
		fmt.Printf("Verify no verify\n")
		return false, nil
	}

}

func VerifyQuote(to_quote []byte, quote_key_info QuoteKeyInfoMessage,
		hash_alg_id uint16, quote_struct_blob []byte,
		signature []byte) (bool) {
	// Decode attest
	attest, err := UnmarshalCertifyInfo(quote_struct_blob)
	if err != nil {
		fmt.Printf("UnmarshalCertifyInfo fails\n")
		return false
	}
	PrintAttestData(attest)

	if attest.Magic_number != ordTpmMagic {
		fmt.Printf("Bad magic number\n")
		return false
	}

	// PCR's valid?
	if !ValidPcr(attest.PcrSelect, attest.PcrDigest) {
		return false
	}

	// Compute quote
	quote_hash, err := ComputeHashValue(hash_alg_id, quote_struct_blob)
	if err != nil {
		fmt.Printf("ComputeHashValue fails")
		return false
	}

	// Get quote key from quote_key_info
	if *quote_key_info.PublicKey.KeyType != "rsa" {
		fmt.Printf("Bad key type %s\n", quote_key_info.PublicKey.KeyType)
		return false;
	}

	// Verify quote
	var N *big.Int
	var E *big.Int
	N  = new(big.Int)
	N.SetBytes(quote_key_info.PublicKey.RsaKey.Modulus)
	E  = new(big.Int)
	E.SetBytes([]byte{0,1,0,1})
	x := new(big.Int)
	x.SetBytes(signature)
	z := new(big.Int)
	z = z.Exp(x, E, N)
	decrypted_quote := z.Bytes()
	start_quote_blob := len(decrypted_quote) - SizeHash(hash_alg_id)
	if bytes.Compare(decrypted_quote[start_quote_blob:], quote_hash) != 0 {
		fmt.Printf("Compare fails.  %x %x\n", quote_hash, decrypted_quote[start_quote_blob:])

		return false
	}
	return true
}

// ConstructInternalMakeCredential constructs a InternalMakeCredential command.
func ConstructInternalMakeCredential(protectorHandle Handle, credential []byte,
		activeName []byte) ([]byte, error) {
	cmdHdr, err := MakeCommandHeader(tagNO_SESSIONS, 0, cmdMakeCredential)
	if err != nil {
		return nil, errors.New("ConstructInternalMakeCredential failed")
	}
	b1 := SetHandle(protectorHandle)
	b2, _ := pack([]interface{}{&credential, activeName})
	cmd_bytes := packWithBytes(cmdHdr, append(b1, b2...))
	return cmd_bytes, nil
}

// DecodeInternalMakeCredential decodes a InternalMakeCredential response.
// returns blob, encrypted_secret
func DecodeInternalMakeCredential(in []byte) ([]byte, []byte, error) {
	var credBlob []byte
	var encrypted_secret []byte

	template :=  []interface{}{&credBlob, &encrypted_secret}
	err := unpack(in, template)
	if err != nil {
		return nil, nil, errors.New("Can't decode InternalMakeCredential response")
	}
	return credBlob, encrypted_secret, nil
}

// InternalMakeCredential
// 	Output: blob, secret
func InternalMakeCredential(rw io.ReadWriter, protectorHandle Handle, credential []byte,
		activeName []byte) ([]byte, []byte, error) {
	// Construct command
	cmd, err:= ConstructInternalMakeCredential (protectorHandle, credential, activeName)
	if err != nil {
		return nil, nil, errors.New("ConstructInternalMakeCredential fails")
	}

	// Send command
	_, err = rw.Write(cmd)
	if err != nil {
		return nil, nil, errors.New("Write Tpm fails")
	}

	// Get response
	var resp []byte
	resp = make([]byte, 2048, 2048)
	read, err := rw.Read(resp)
	if err != nil {
		return nil, nil, errors.New("Read Tpm fails")
	}

	// Decode Response
	if read < 10 {
		return nil, nil, errors.New("Read buffer too small")
	}
	_, _, status, err := DecodeCommandResponse(resp[0:10])
	if err != nil {
		return nil, nil, errors.New("DecodeCommandResponse fails")
	}
	if status != ErrSuccess {
		return nil, nil, errors.New("InternalMakeCredential unsuccessful")
	}
	credBlob, encrypted_secret, err := DecodeInternalMakeCredential(resp[10:])
	if err != nil {
		return nil, nil, errors.New("DecodeInternalMakeCredential fails")
	}
	return credBlob, encrypted_secret, nil
}

func MarshalRsaPrivateToProto(key *rsa.PrivateKey) (*RsaPrivateKeyMessage, error) {
	if key == nil {
		return nil, errors.New("No key")
	}
	msg := new(RsaPrivateKeyMessage)
	msg.PublicKey = new(RsaPublicKeyMessage)
	msg.D = key.D.Bytes()
	msg.PublicKey.Exponent = []byte{0,1,0,1}
	msg.PublicKey.Modulus = key.N.Bytes()
	l := int32(len(msg.PublicKey.Modulus) * 8)
	msg.PublicKey.BitModulusSize = &l
	// if len(key.Primes == 2 {
	// 	msg.PublicKey.P = msg.Primes[0].Bytes()
	// 	msg.PublicKey.Q = msg.Primes[1].Bytes()
	// }
	return msg, nil
}

func UnmarshalRsaPrivateFromProto(msg *RsaPrivateKeyMessage) (*rsa.PrivateKey, error) {
	if msg == nil {
		return nil, errors.New("No message")
	}
	key := new(rsa.PrivateKey)
	// key.PublicKey = new(rsa.PublicKey)
	key.D = new(big.Int)
	key.D.SetBytes(msg.D)
	key.PublicKey.N = new(big.Int)
	key.PublicKey.N.SetBytes(msg.PublicKey.Modulus)
	key.PublicKey.E = 0x10001  // Fix
	// if msg.PublicKey.P != nil && msg.PublicKey.Q != nil {
	// 	msg.Primes[0] = new(big.Int)
	// 	msg.Primes[1] = new(big.Int)
	// 	msg.Primes[0].SetBytes(msg.PublicKey.P)
	// 	msg.Primes[1].SetBytes(msg.PublicKey.Q)
	// }
	return key, nil
}


// Input: Der encoded endorsement cert and handles
// Returns program private key protobuf, CertRequestMessage
func ConstructClientRequest(rw io.ReadWriter, der_endorsement_cert []byte,
		quote_handle Handle, parent_pw string, owner_pw string,
		program_name string) (*RsaPrivateKeyMessage,
		*ProgramCertRequestMessage, error) {

	// Generate Program Key.
	programPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Printf("Can't generate program key\n")
		return nil, nil, err
	}
	fmt.Printf("Generated private key: %x\n", programPrivateKey)
	privateKeyMsg, err := MarshalRsaPrivateToProto(programPrivateKey)
	if err != nil {
		fmt.Printf("Can't marshal key to proto\n")
		return nil, nil, err
	}
	programPublicKey := programPrivateKey.PublicKey
	fmt.Printf("exp: %x\n", programPublicKey.E)
	fmt.Printf("mod: %x\n", programPublicKey.N)
	fmt.Printf("mod bytes: %x\n", programPublicKey.N.Bytes())

	// Generate Request
	request := new(ProgramCertRequestMessage)
	request.ProgramKey = new(ProgramKeyParameters)
	request.EndorsementCertBlob = der_endorsement_cert
	req_id := "001"
	request.RequestId = &req_id
	modulus_bits := int32(2048)
	key_type := "rsa"
	fmt.Printf("Program name: %s\n", program_name)
	request.ProgramKey.ProgramName =  &program_name
	request.ProgramKey.ProgramKeyType = &key_type
	request.ProgramKey.ProgramBitModulusSize = &modulus_bits
	fmt.Printf("Request generated\n")

	request.ProgramKey.ProgramKeyExponent =  []byte{0,1,0,1}
	request.ProgramKey.ProgramKeyModulus = programPublicKey.N.Bytes()
	serialized_program_key := proto.CompactTextString(request.ProgramKey)
	sha1Hash := sha1.New()
	sha1Hash.Write([]byte(serialized_program_key))
	hashed_program_key := sha1Hash.Sum(nil)
	fmt.Printf("ProgramKey: %s\n", serialized_program_key)

	// Quote key
	key_blob, quote_key_name, _, err := ReadPublic(rw, quote_handle)
	if err != nil {
		return nil, nil, err
	}
	rsaQuoteParams, err := DecodeRsaBuf(key_blob)
	if err != nil {
		fmt.Printf("Can't decode quote blob\n")
		return nil, nil, err
	}
	PrintRsaParams(rsaQuoteParams)
	fmt.Printf("Quote key name: %x\n", quote_key_name)
	fmt.Printf("parent_pw: %s, owner_pw: %s\n", parent_pw, owner_pw)

	fmt.Printf("Generating quote\n")
	sig_alg := uint16(AlgTPM_ALG_NULL)
	attest, sig, err := Quote(rw, quote_handle, owner_pw, owner_pw,
		hashed_program_key, []int{7}, sig_alg)
	if err != nil {
		fmt.Printf("Quote failed", err, "\n")
		return nil, nil, err
	}
	fmt.Printf("Attest: %x\n", attest)
	fmt.Printf("Sig: %x\n", sig)

	// Quote key info.
	request.QuoteKeyInfo = new(QuoteKeyInfoMessage)
	request.QuoteKeyInfo.Name = quote_key_name
	tmp_name := "Quote-Key"
	request.QuoteKeyInfo.PublicKey= new(PublicKeyMessage)
	request.QuoteKeyInfo.PublicKey.RsaKey = new(RsaPublicKeyMessage)
	request.QuoteKeyInfo.PublicKey.RsaKey.KeyName = &tmp_name
	var enc_alg string
	var hash_alg string
	if  rsaQuoteParams.Enc_alg == AlgTPM_ALG_RSA {
		enc_alg = "rsa"
	} else {
		fmt.Printf("Unsupported enc alg\n")
		return nil, nil, err
	}
	if  rsaQuoteParams.Hash_alg == AlgTPM_ALG_SHA1 {
		hash_alg = "sha1"
	} else if  rsaQuoteParams.Hash_alg == AlgTPM_ALG_SHA256 {
		hash_alg = "sha256"
	} else {
		fmt.Printf("Unsupported hash alg\n")
		return nil, nil, err
	}
	request.QuoteKeyInfo.PublicKey.KeyType = &enc_alg
	size := int32(rsaQuoteParams.Mod_sz)
	request.QuoteKeyInfo.PublicKey.RsaKey.BitModulusSize =  &size
	request.QuoteKeyInfo.PublicKey.RsaKey.Modulus =  rsaQuoteParams.Modulus
	request.QuoteSignAlg = &enc_alg
	request.QuoteSignHashAlg = &hash_alg

	request.ProgramKey = new(ProgramKeyParameters)
	request.ProgramKey.ProgramName = &program_name
	request.ProgramKey.ProgramKeyType= &enc_alg
	request.ProgramKey.ProgramBitModulusSize= &modulus_bits
	request.ProgramKey.ProgramKeyModulus = programPublicKey.N.Bytes()

	request.QuotedBlob = attest
	request.QuoteSignature = sig
	return privateKeyMsg, request, nil
}

// Input: policy private key
func ConstructServerResponse(policy_private_key *rsa.PrivateKey, der_policy_cert []byte,
	     signing_instructions_message SigningInstructionsMessage,
	     request ProgramCertRequestMessage) (*ProgramCertResponseMessage, error) {

	if request.ProgramKey == nil {
		fmt.Printf("program key is nil\n")
	}
	// hash program key
	serialized_program_key := proto.CompactTextString(request.ProgramKey)
	sha256Hash := sha256.New()
	sha256Hash.Write([]byte(serialized_program_key))
	hashed_program_key := sha256Hash.Sum(nil)

	var hash_alg_id uint16
	if *request.QuoteSignHashAlg == "sha256" {
		hash_alg_id = uint16(AlgTPM_ALG_SHA256)
	} else {
		hash_alg_id = uint16(AlgTPM_ALG_SHA1)
	}
	if !VerifyQuote(hashed_program_key, *request.QuoteKeyInfo, hash_alg_id,
			request.QuotedBlob, request.QuoteSignature) {
		return nil, errors.New("Can't verify quote")
	}

	// Create Program Key Certificate	
	progName := request.ProgramKey.ProgramName
	var notBefore time.Time
	notBefore = time.Now()
	validFor := 365*24*time.Hour
	notAfter := notBefore.Add(validFor)
	template := x509.Certificate{
		SerialNumber: GetSerialNumber(),
		Subject: pkix.Name {
			Organization: []string{"CloudProxyAuthority"},
			CommonName:   *progName,
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	pub := new(rsa.PublicKey)
	m := new(big.Int)
	m.SetBytes(request.ProgramKey.ProgramKeyModulus)
	pub.N = m
	pub.E = 0x00010001
	der_program_cert, err := x509.CreateCertificate(rand.Reader, &template, &template,
		pub, policy_private_key)
	if err != nil {
		fmt.Printf("Can't create certificates\n")
		return nil, err
	}

	// Get Endorsement blob
	endorsement_cert, err := x509.ParseCertificate(request.EndorsementCertBlob)
	if err !=nil {
		fmt.Printf("Can't Parse endorsement cert\n")
		return nil, err
	}

	// Verify Endorsement Cert
	ok, err := VerifyDerCert(request.EndorsementCertBlob, der_policy_cert)
	if !ok {
		fmt.Printf("Bad endorsement cert")
		return nil, errors.New("Bad endorsement cert")
	}

	var protectorPublic *rsa.PublicKey
	switch k :=  endorsement_cert.PublicKey.(type) {
	case  *rsa.PublicKey:
		protectorPublic = k
	case  *rsa.PrivateKey:
		protectorPublic = &k.PublicKey
	default:
		fmt.Printf("endorsement cert is not an rsa key\n")
		return nil, errors.New("endorsement cert not an rsa key")
	}

	// Generate credential
	var credential [16]byte
	rand.Read(credential[0:16])
	fmt.Printf("Credential: %x, hashid: %x\n", credential, hash_alg_id)
	fmt.Printf("Name: %x\n", request.QuoteKeyInfo.Name)
	encrypted_secret, encIdentity, integrityHmac, err := MakeCredential(
		protectorPublic, hash_alg_id,
		credential[0:16], request.QuoteKeyInfo.Name)
	if err != nil {
		fmt.Printf("MakeCredential fails\n")
		return nil, err
	}

	// Response
	response := new(ProgramCertResponseMessage)
	response.RequestId = request.RequestId
	response.ProgramName = progName
	integrity_alg := *request.QuoteSignHashAlg
	response.Secret = encrypted_secret
	response.IntegrityAlg = &integrity_alg
	response.IntegrityHMAC = integrityHmac
	response.EncIdentity = encIdentity

	// Encrypt cert with credential
	cert_hmac, cert_out, err :=  EncryptDataWithCredential(true, hash_alg_id, 
		credential[0:16], der_program_cert, nil)
	if err != nil {
		return nil, err
	}
	response.EncryptedCert = cert_out
	response.EncryptedCertHmac = cert_hmac
	return response, nil
}

// Output is der encoded Program Cert
func ClientDecodeServerResponse(rw io.ReadWriter, protectorHandle Handle,
		quoteHandle Handle, password string,
		response ProgramCertResponseMessage) ([]byte, error) {
	certBlob := append(response.IntegrityHMAC, response.EncIdentity...)
	certInfo, err := ActivateCredential(rw, quoteHandle, protectorHandle, password, "",
		certBlob, response.Secret)
	if err != nil {
		fmt.Printf("ActivateCredential failed ", err, "\n")
		return nil, err
	}
	fmt.Printf("certInfo: %x\n", certInfo)

	// Decrypt cert.
	_, out, err :=  EncryptDataWithCredential(false, uint16(AlgTPM_ALG_SHA1),
		certInfo, response.EncryptedCert, response.EncryptedCertHmac)
	if err != nil {
		fmt.Printf("EncryptDataWithCredential failed\n")
		return nil, err
	}
	return out, nil
}
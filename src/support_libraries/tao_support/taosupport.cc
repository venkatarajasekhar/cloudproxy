//  Copyright (c) 2014, Google Inc.  All rights reserved.
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
#include <string>
#include <vector>
#include <stdlib.h>

#include <gflags/gflags.h>
#include <glog/logging.h>

#include "tao/fd_message_channel.h"
#include "tao/tao_rpc.h"
#include "tao/util.h"

#include "helpers.h"
#include "taosupport.h"

#include <openssl/ssl.h>
#include <openssl/rsa.h>
#include <openssl/x509.h>
#include <openssl/x509v3.h>
#include <openssl/rand.h>

#include "keys.pb.h"
#include "ca.pb.h"
#include "domain_policy.pb.h"
#include "auth.h"

using std::string;
using std::unique_ptr;

using tao::Base64WDecode;
using tao::Base64WEncode;
using tao::FDMessageChannel;
using tao::InitializeApp;
using tao::MarshalSpeaksfor;
using tao::Tao;
using tao::TaoRPC;

#include <google/protobuf/io/coded_stream.h>
#include <google/protobuf/io/zero_copy_stream_impl_lite.h>
#include <google/protobuf/stubs/common.h>
using google::protobuf::io::CodedInputStream;
using google::protobuf::io::CodedOutputStream;
using google::protobuf::io::StringOutputStream;
using google::protobuf::io::ArrayInputStream;

#define BUFSIZE 8192

void SerializeTermToString(tao::Term* term, string* name) {
  if (dynamic_cast<tao::Prin*> (term)) {
    tao::Prin* prin = dynamic_cast<tao::Prin*>(term);
    *name += prin->type_ + "("; 
    SerializeTermToString(prin->keyhash_.get(), name);
    *name += ")";
    tao::SubPrin* w = prin->ext_.get();
    for (std::vector<std::unique_ptr<tao::PrinExt>>::iterator
           it = w->elts_.begin(); it != w->elts_.end(); ++it) {
      *name += ".";
      tao::PrinExt* prinExt = (*it).get();
      *name += prinExt->name_ + "(";
      SerializeTermToString(prinExt->args_[0].get(), name);
      *name += ")";
    }
  } else if (dynamic_cast<tao::Bytes*> (term)) {
    tao::Bytes* bytes = dynamic_cast<tao::Bytes*> (term);
    string* hex = ByteToHexLeftToRight((int)bytes->elt_.size(), (byte*)bytes->elt_.data());
    *name += *hex;
    delete hex;
  }
}

TaoChannel::TaoChannel() {
  peerCertificate_ = nullptr;
}

TaoChannel::~TaoChannel() {
}

bool TaoChannel::OpenTaoChannel(TaoProgramData& client_program_data,
                    string& serverAddress, string& port) {

  // Parse policy cert and program cert.
  string policy_cert;
  if (!client_program_data.GetPolicyCert(&policy_cert) ||
       policy_cert.size() == 0 ) {
    printf("No policy cert.\n");
    return false;
  }
  X509* policyCertificate = nullptr;
  byte* pc = (byte*)policy_cert.data();
  policyCertificate = d2i_X509(nullptr,
      (const byte**)&pc, policy_cert.size());
  if (policyCertificate == nullptr) {
    printf("Can't parse policy certificate.\n");
    return false;
  }
  client_program_data.SetPolicyCertificate(policyCertificate);

  string program_cert;
  if (!client_program_data.GetProgramCert(&program_cert) ||
       program_cert.size() == 0 ) {
    printf("No program certificate.\n");
    return false;
  }

  pc = (byte*)program_cert.data();
  X509* programCertificate= d2i_X509(nullptr, (const byte**)&pc,
        program_cert.size());
  if (programCertificate == nullptr) {
    printf("Can't translate program certificate.\n");
    return false;
  }
  client_program_data.SetProgramCertificate(programCertificate);
  if (client_program_data.GetProgramKey() == nullptr) {
      printf("No program private key.\n");
      return false;
  }

  string key_type;
  if (!client_program_data.GetProgramKeyType(&key_type)) {
      printf("No private key type.\n");
      return false;
  }

  // Open TLS channel with Program cert.
  string network("tcp");
  if (!peer_channel_.InitClientSslChannel(network, serverAddress, port,
                    client_program_data.GetPolicyCertificate(),
                    client_program_data.GetProgramCertificate(),
                    key_type,
                    client_program_data.GetProgramKey(),
                    SSL_SERVER_VERIFY_CLIENT_VERIFY)) {
    printf("Can't Init Ssl channel.\n");
    return false;
  }

  // Get peer name from organizational unit.
  peerCertificate_ = peer_channel_.GetPeerCert();
  if (peerCertificate_ != nullptr) {
    X509_NAME* name = X509_get_subject_name(peerCertificate_);
    int nid = OBJ_txt2nid("OU");
    char buf[BUFSIZE];
    if (X509_NAME_get_text_by_NID(name, nid, buf, BUFSIZE) == 1) {
      peer_name_ = buf ;
    }
  }
  return true;
}

void TaoChannel::CloseTaoChannel() {
  peer_channel_.Close();
}

bool TaoChannel::SendRequest(int size, byte* out) {
  int k = SslMessageWrite(peer_channel_.GetSslChannel(), size, out);
  return k > 0;
}

bool TaoChannel::GetRequest(int* size, byte* in) {
  int k = SslMessageRead(peer_channel_.GetSslChannel(), *size, in);
  if (k <= 0) {
    printf("Can't read request channel.\n");
    return false;
  }
  *size = k;
  return true;
}

TaoProgramData::TaoProgramData() {
  initialized_ = false;
  tao_ = nullptr;
  program_key_type_.clear();
  program_key_ = nullptr;
  size_program_sym_key_ = 0;
  program_sym_key_ = nullptr;
  programCertificate_ = nullptr;
  policyCertificate_ = nullptr;
}

TaoProgramData::~TaoProgramData() {
  ClearProgramData();
}

void TaoProgramData::SetPolicyCertificate(X509* c) {
  policyCertificate_ = c;
}

void TaoProgramData::SetProgramCertificate(X509* c) {
  programCertificate_ = c;
}

bool TaoProgramData::GetTaoName(string* name) {
  if (!initialized_)
    return false;
  *name = tao_name_;
  return true;
}

bool TaoProgramData::GetSymKeys(string* symkeys) {
  if (!initialized_)
    return false;
  symkeys->assign((const char*)program_sym_key_, size_program_sym_key_);
  return true;
}

bool TaoProgramData::GetPolicyCert(string* cert) {
  if (!initialized_)
    return false;
  *cert = policy_cert_;
  return true;
}

X509* TaoProgramData::GetPolicyCertificate() {
  if (!initialized_)
    return nullptr;
  return policyCertificate_;
}

bool TaoProgramData::GetProgramKeyType(string* keyType) {
  if (!initialized_)
    return false;
  *keyType = program_key_type_;
  return true;
}

EVP_PKEY* TaoProgramData::GetProgramKey() {
  if (!initialized_)
    return nullptr;
  return program_key_;
}

bool TaoProgramData::GetProgramCert(string* cert) {
  *cert = program_cert_;
  return true;
}

X509* TaoProgramData::GetProgramCertificate() {
  return programCertificate_;
}

std::list<string>* TaoProgramData::GetCertChain() {
  if (!initialized_)
    return nullptr;
  return &certs_in_chain_;
}

void TaoProgramData::ClearProgramData() {
  initialized_ = false;
  marshalled_tao_name_.clear();
  tao_name_.clear();
  policy_cert_.clear();

  tao_ = nullptr;

  // TODO: erase key first.
  // Clear private key.
  if (program_key_ != nullptr) {
    EVP_PKEY_free(program_key_);
  }
  program_key_ = nullptr;

  if (size_program_sym_key_ > 0 && program_sym_key_ != nullptr) {
    memset(program_sym_key_, 0, size_program_sym_key_);
    free(program_sym_key_);
  }

  if (policyCertificate_ != nullptr) {
    X509_free(policyCertificate_);
  }
  policyCertificate_ = nullptr;
  if (programCertificate_ != nullptr) {
    X509_free(programCertificate_);
  }
  programCertificate_ = nullptr;
}

bool TaoProgramData::InitTao(FDMessageChannel* msg, Tao* tao, string& cfg,
       string& path, string& network, string& address, string& port, string& cipher_suite,
       bool useSimpleService) {

  // Set tao and msg for later calls.
  msg_ = msg;
  tao_ = tao;

  // Read policy cert.
  string policy_cert_file = path + "/policy_keys/cert";
  if (!ReadFile(policy_cert_file, &policy_cert_)) {
    printf("Can't read policy cert.\n");
    return false;
  }

  // Parse policy cert.
  byte* pc = (byte*)policy_cert_.data();
  X509* parsed_policy_cert = d2i_X509(nullptr, (const byte**)&pc,
          policy_cert_.size());
  if (parsed_policy_cert == nullptr) {
    printf("Can't DER parse policy cert.\n");
    return false;
  }

  string keyType;
  int key_size;
  EVP_PKEY* evp_policy_key = X509_get_pubkey(parsed_policy_cert);
  if (evp_policy_key == nullptr) {
    printf("Can't get policy public key from cert.\n");
    return false;
  }
  int key_type = EVP_PKEY_id(evp_policy_key);
  if (EVP_PKEY_EC == key_type) {
    keyType = "ECC";
    key_size = 256;
  } else if (EVP_PKEY_RSA == key_type) {
    keyType = "RSA";
    key_size = 2048;
  } else {
    printf("Unsupported key type.\n");
    return false;
  }


  // Extend principal name, with hash of policy public key.

  // Hash of policy cert.
  string policy_hash_str;

  if(!GetKeyBytes(evp_policy_key, &policy_hash_str)) {
    printf("Can't get key bytes.\n");
    return false;
  }

  std::vector<std::unique_ptr<tao::PrinExt>> v;

  std::vector<std::unique_ptr<tao::Term>> w;
  w.push_back(tao::make_unique<tao::Bytes>(policy_hash_str.data()));
  v.push_back(tao::make_unique<tao::PrinExt> ("PolicyKey", std::move(w)));
  tao::SubPrin p(std::move(v));
  string subprin;
  {
    StringOutputStream raw_output_stream(&subprin);
    CodedOutputStream output_stream(&raw_output_stream);
    p.Marshal(&output_stream);
  }

  if (!tao_->ExtendTaoName(subprin)) {
    printf("Can't extend name.\n");
    return false;
  }

  // Retrieve extended name.
  if (!tao->GetTaoName(&marshalled_tao_name_)) {
    printf("Can't get tao name.\n");
    return false;
  }

  tao::Prin unmarshalled_tao_name;
  {
    ArrayInputStream raw_input_stream(marshalled_tao_name_.data(),
                                      marshalled_tao_name_.size());
    CodedInputStream input_stream(&raw_input_stream);
    if (!unmarshalled_tao_name.Unmarshal(&input_stream)) {
        printf("Can't unmarshal tao name\n");
    }
  }
  SerializeTermToString((tao::Term*)&unmarshalled_tao_name, &tao_name_);

  // Get (or initialize) my symmetric keys.
  if (!InitializeSymmetricKeys(path, 32)) {
    printf("Can't init symmetric keys.\n");
    return false;
  }

  // Get (or initialize) my program key.
  // First, we need the host cert and cert type.
  // TODO: get it from host?
  // If host_type is "tpm" or "tpm2", read the endosement cert as
  // the host cert.
  string host_type("fake");
  string host_cert;

#if 0
  string host_cert_file_name = path + "/endorsementCert";
  if (!ReadFile(host_cert_file_name, &endorse_cert)) {
    printf("InitializeProgramKey: couldn't read host cert.\n");
    return false;
  }
#endif

  if (!InitializeProgramKey(path, keyType, key_size, network, address,
          port, host_type, host_cert)) {
    printf("Can't init program keys.\n");
    return false;
  }
  initialized_ = true;
  return true;
}

void TaoProgramData::Print() {
  if (!initialized_) {
    printf("Program object is NOT initialized\n");
    return;
  }
  printf("Program object is NOT initialized\n");
  printf("Tao name: %s\n", marshalled_tao_name_.c_str());
  printf("Policy cert: ");
  PrintBytes(policy_cert_.size(), (byte*)policy_cert_.data());printf("\n");
  printf("Program key: "); printf("TODO"); printf("\n");
  printf("Sym key: ");PrintBytes(size_program_sym_key_, program_sym_key_);printf("\n");
  printf("Program cert: ");PrintBytes(program_cert_.size(), (byte*)program_cert_.data());printf("\n");
  printf("Program path: %s\n", program_file_path_.c_str());
}

void TaoChannel::Print() {
  printf("Peer name: %s\n", peer_name_.c_str());
}

bool TaoProgramData::Attest(string& to_attest, string* attested) {
  return tao_->Attest(to_attest, attested);
}

bool TaoProgramData::Seal(string& data, string* sealed) {
  return tao_->Seal(data, Tao::SealPolicyDefault, sealed);
}

bool TaoProgramData::Unseal(string& sealed, string* unsealed) {
  string policy;
  return tao_->Unseal(sealed, unsealed, &policy);
}

bool TaoProgramData::InitCounter(string& label, int64_t& c) {
printf("Calling tao_->TaoProgramData::InitCounter(%llx)\n", tao_);
  return tao_->InitCounter(label, c);
}

bool TaoProgramData::GetCounter(string& label, int64_t* c) {
  return tao_->GetCounter(label, c);
}

bool TaoProgramData::RollbackProtectedSeal(string& label, string& data, string* sealed) {
  return tao_->RollbackProtectedSeal(label, data, Tao::SealPolicyDefault, sealed);
}

bool TaoProgramData::RollbackProtectedUnseal(string& sealed, string* data, string* policy) {
  return tao_->RollbackProtectedUnseal(sealed, data, policy);
}

bool TaoProgramData::RequestDomainServiceCert(string& network, string& address,
                              string& port, string& attestation_string,
                              string& endorsement_cert,
                              string* program_cert,
                              std::list<string>* certChain) {

  if (policyCertificate_ == nullptr) {
    printf("Policy cert is null.\n");
    return false;
  }

  X509_REQ* req = X509_REQ_new();;
  X509* cert = X509_new();
  string key_type("ECC");
  string common_name("Fred");
  string issuer("Self");
  string keyUsage("critical,digitalSignature,keyEncipherment,keyAgreement,keyCertSign");
  string extendedKeyUsage("serverAuth,clientAuth");

  EVP_PKEY* self = GenerateKey(key_type, 256);
  if (self == nullptr) {
    printf("Can't Generate temporary channel key.\n");
    return false;
  }
  if (!GenerateX509CertificateRequest(key_type, common_name, self,
         false, req)) {
    printf("Can't generate x509 request\n");
    return false;
  }

  if (!SignX509Certificate(self, true, true, issuer, 
                           keyUsage, extendedKeyUsage, 86400,
                           self, req, false, cert)) {
    printf("Can't sign x509 request\n");
    return false;
  }

  SslChannel domainChannel;

  if (!domainChannel.InitClientSslChannel(network, address, port,
        cert, cert, key_type, self,
        SSL_NO_SERVER_VERIFY_NO_CLIENT_VERIFY)) {
    printf("Can't init ssl channel to domain server.\n");
    return false;
  }

  // Format request and send it to Domain service and get response.
  int bytes_written = SslMessageWrite(domainChannel.GetSslChannel(),
                          (int)attestation_string.size(),
                          (byte*)attestation_string.data());
  if (bytes_written <= 0) {
    printf("Domain channel write failure.\n");
    return false;
  }
  byte read_buf[BUFSIZE];
  string response_buf;
  int bytes_read = 0;
  while ((bytes_read = SslMessageRead(domainChannel.GetSslChannel(), BUFSIZE, read_buf))
           == 0);
  if (bytes_read <= 0) {
    printf("Domain channel read failure (%d).\n", bytes_read);
    return false;
  }

  response_buf.assign((const char*)read_buf, bytes_read);
  domain_policy::DomainCertResponse response;
  if (!response.ParseFromString(response_buf)) {
    printf("Domain channel parse failure.\n");
    return false;
  }
  // Fill in program cert.
  program_cert->assign((const char*)response.signed_cert().data(),
                       response.signed_cert().size());

  // Cert chain
  for (int j = 0; j < response.cert_chain_size(); j++) {
      certChain->push_back(string(response.cert_chain(j)));
  }
  return true;
}

bool TaoProgramData::InitializeSymmetricKeys(string& path, int keysize) {
  string sealed;
  string unsealed;
  string file_name = path + "/sealedsymmetricKey";

  // Read key file.
  if (ReadFile(file_name, &sealed)) {
    if (!Unseal(sealed, &unsealed)) {
      printf("Can't open InitializeSymmetricKeys %s\n", file_name.c_str());
      return false;
    }
    size_program_sym_key_ = unsealed.size();
    program_sym_key_ = (byte*)malloc(size_program_sym_key_);
    memcpy(program_sym_key_, (byte*)unsealed.data(), size_program_sym_key_ );
    memset((byte*)unsealed.data(), 0, size_program_sym_key_);
    return true;
  }

  // Create keys, should really be a call to GetRandom.
  program_sym_key_ = (byte*)malloc(keysize);
  if (program_sym_key_ == nullptr) {
    printf("InitializeSymmetricKeys: Can't malloc symmetric key.\n");
    return false;
  }
  size_program_sym_key_ = keysize;
  if (1 != RAND_bytes(program_sym_key_, keysize)) {
    printf("InitializeSymmetricKeys: Can't generate symmetric key %d.\n",
           keysize);
    return false;
  }

  // Seal the key and save it.
  unsealed.assign((const char*)program_sym_key_, size_program_sym_key_);
  string policy = Tao::SealPolicyDefault;
  if (!tao_->Seal(unsealed, policy, &sealed)) {
    unsealed.clear();
    printf("InitializeSymmetricKeys: Can't seal sym keys\n");
    return false;
  }
  if (!WriteFile(file_name, sealed)) {
    printf("InitializeSymmetricKeys: write sealed sym keys.\n");
    return false;
  }
  unsealed.clear();
  return true;
}

bool TaoProgramData::InitializeProgramKey(string& path, string& key_type,
        int key_size, string& network, string& address, string& port,
        string& host_type, string& host_cert) {
  string sealed_key_file_name = path + "/sealedsigningKey";
  string signer_cert_file_name = path + "/signerCert";
  string policy_cert_file_name = path + "/policy_keys/cert";
  string sealed_key;
  string unsealed_key;

  // Read and parse policy cert.
  if (!ReadFile(policy_cert_file_name, &policy_cert_)) {
    printf("InitializeProgramKey: Can't read policy cert.\n");
    return false;
  }
  byte* pc = (byte*)policy_cert_.data();
  policyCertificate_ = d2i_X509(nullptr, (const byte**)&pc,
        policy_cert_.size());
  if (policyCertificate_ == nullptr) {
    printf("InitializeProgramKey: policy certificate is null.\n");
    return false;
  }

  if (ReadFile(sealed_key_file_name, &sealed_key) &&
      ReadFile(signer_cert_file_name, &program_cert_)) {
    if (!Unseal(sealed_key, &unsealed_key)) {
      printf("InitializeProgramKey: Can't open InitializeProgramKey\n");
      return false;
    }
    // Deserialize the key.
    if (!DeserializePrivateKey(unsealed_key, &program_key_type_,
             &program_key_)) {
      printf("InitializeProgramKey: Can't DeserializePrivateKey\n");
      return false;
    }
    return true;
  }

  // Generate the key and specify key bytes.
  program_key_ = GenerateKey(key_type, key_size);
  if (program_key_ == nullptr) {
    printf("InitializeProgramKey: couldn't generate program key.\n");
    return false;
  }
  string key_bytes;
  if (!GetKeyBytes(program_key_, &key_bytes)) {
    printf("InitializeProgramKey: couldn't get key bytes.\n");
    return false;
  }

  // Get the program cert from the domain service.
#if 0
  // First, we need the endorsement cert,
  // if parent is a tpm.
  string endorsement_cert_file_name = path + "/endorsementCert";
  string endorse_cert;
  if (!ReadFile(endorsement_cert_file_name, &endorse_cert)) {
    printf("InitializeProgramKey: couldn't read endorsement cert.\n");
    return false;
  }
#endif

  // Construct a delegation statement.
  string msf;
  if (!MarshalSpeaksfor(key_bytes, marshalled_tao_name_, &msf)) {
    printf("InitializeProgramKey: couldn't MarshalSpeaksfor.\n");
    return false;
  }

  // Get an attestation using delegation and program key;
  string attestation_string;
  if (!Attest(msf, &attestation_string)) {
    printf("InitializeProgramKey: couldn't Attest.\n");
    return false;
  }

  // Der serialize key
  byte out[4096];
  byte* ptr = out;
  int n = i2d_PUBKEY(program_key_, &ptr);
  if (n <= 0) {
    printf("Can't i2d ECC public key\n");
    return false;
  }

  // Make cert request.
  domain_policy::DomainCertRequest request;
  request.set_attestation(attestation_string);
  request.set_key_type("ECDSA");
  request.set_subject_public_key(out, n);
  // printf("Der program key: ");PrintBytes(n, out); printf("\n");

  string request_string;
  if (!request.SerializeToString(&request_string)) {
    printf("InitializeProgramKey: couldn't serialize request.\n");
    return false;
  }

  // Get Program Cert.
  // if (!RequestDomainServiceCert(network, address, port, attestation_string,
  if (!RequestDomainServiceCert(network, address, port, request_string,
          host_cert, &program_cert_, &certs_in_chain_)) {
    printf("InitializeProgramKey: couldn't RequestDomainServiceCert.\n");
    return false;
  }

  // Save the program cert.
  if (!WriteFile(signer_cert_file_name, program_cert_)) {
    printf("InitializeProgramKey: couldn't write signed program cert.\n");
    return false;
  }

  // Serialize Key.
  program_key_type_ = key_type;
  string out_buf;
  if (!SerializePrivateKey(program_key_type_, program_key_, &out_buf)) {
    printf("InitializeProgramKey: couldn't serialize private key.\n");
    return false;
  }

  // Seal the key and save it.
  string sealed_out;
  string policy = Tao::SealPolicyDefault;
  if (!tao_->Seal(out_buf, policy, &sealed_out)) {
    out_buf.clear();
    printf("InitializeProgramKeys: Can't seal program key\n");
    return false;
  }
  if (!WriteFile(sealed_key_file_name, sealed_out)) {
    printf("InitializeProgramKey: couldn't write sealed private key.\n");
    return false;
  }
  return true;
}

// For ec name, KeyBytes should be marshalled version of:
//   enum NamedEllipticCurve { PRIME256_V1 = 1;}
//   ECDSA_SHA_VerifyingKeyV1
//     Curve:    NamedEllipticCurve_PRIME256_V1.Enum(),
//     EcPublic: elliptic.Marshal(k.Curve, k.X, k.Y),
// Points marshalled as in section 4.3.6 of ANSI X9.62.

#pragma pack(push, 1)
struct ecMarshal {
  byte compress_;
  byte X_[32];
  byte Y_[32];
};
#pragma pack(pop)

bool GetKeyBytes(EVP_PKEY* pKey, string* bytes_out) {
  string key_bytes;
  byte key_hash[32];
  byte out[4096];
  byte* ptr = out;
  int n;

  if (pKey->type == EVP_PKEY_RSA) {
    RSA* rsa_key = EVP_PKEY_get1_RSA(pKey);
    // FIX: change to however Rsa keys are serialized internally.
    n = i2d_RSA_PUBKEY(rsa_key, &ptr);
    if (n <= 0) {
      printf("GetKeyBytes: Can't i2d RSA public key\n");
      return false;
    }
    SHA256_CTX sha256;
    SHA256_Init(&sha256);
    SHA256_Update(&sha256, out, n);
    SHA256_Final(key_hash, &sha256);
    bytes_out->assign((const char*)key_hash, 32);
  } else if (pKey->type == EVP_PKEY_EC) {
    // Use get0?
    EC_KEY* ec_key = EVP_PKEY_get1_EC_KEY(pKey);

    ecMarshal ec_params;
    ec_params.compress_ = 4;
    BN_CTX* bn_ctx = BN_CTX_new();
    if (bn_ctx == nullptr) {
      printf("Can't get BN_CTX\n");
      return false;
    }
    BIGNUM* x = BN_new();
    BIGNUM* y = BN_new();
    string vk_proto;
    string ec_params_str;

    // Get curve, X and Y
    const EC_POINT* public_point = EC_KEY_get0_public_key(ec_key);
    const EC_GROUP* group = EC_KEY_get0_group(ec_key);
    if (1 != EC_POINT_get_affine_coordinates_GFp(group, public_point, x, y, bn_ctx)) {
      printf("Can't EC_POINT_get_affine_coordinates_GFp\n");
      return false;
    }
    string x_str;
    string y_str;
    if (!BN_to_string(*x, &x_str)) {
      printf("Can't convert x\n");
      return false;
    }
    if (!BN_to_string(*y, &y_str)) {
      printf("Can't convert y\n");
      return false;
    }
    memcpy(ec_params.X_, (byte*)x_str.data(), x_str.size());
    memcpy(ec_params.Y_, (byte*)y_str.data(), y_str.size());
    BN_CTX_free(bn_ctx); BN_free(x); BN_free(y);

    // set and marshal verifying key
    tao::ECDSA_SHA_VerifyingKey_v1 vk;
    vk.set_curve(tao::NamedEllipticCurve::PRIME256_V1);
    ec_params_str.assign((const char*) &ec_params, sizeof(ec_params));
    vk.set_ec_public(ec_params_str);
    vk.SerializeToString(&vk_proto);

    // set and marshal cryptokey
    tao::CryptoKey ck;
    ck.set_version(ck.version());  // crypto version
    ck.set_purpose(tao::CryptoKey_CryptoPurpose::CryptoKey_CryptoPurpose_VERIFYING);
    ck.set_algorithm(tao::CryptoKey_CryptoAlgorithm::CryptoKey_CryptoAlgorithm_ECDSA_SHA);
    ck.set_key(vk_proto);
    ck.SerializeToString(&key_bytes);
    SHA256_CTX sha256;
    SHA256_Init(&sha256);
    SHA256_Update(&sha256, key_bytes.data(), key_bytes.size());
    SHA256_Final(key_hash, &sha256);
    bytes_out->assign((const char*)key_hash, 32);
  } else {
    printf("GetKeyBytes: unsupported key type.\n");
    return false;
  }
  return true;
}


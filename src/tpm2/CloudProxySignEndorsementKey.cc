#include <stdio.h>
#include <stdlib.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <unistd.h>
#include <string.h>

#include <tpm20.h>
#include <tpm2_lib.h>
#include <gflags/gflags.h>

//
// Copyright 2015 Google Corporation, All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// or in the the file LICENSE-2.0.txt in the top level sourcedirectory
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License
//
// Portions of this code were derived TPM2.0-TSS published
// by Intel under the license set forth in intel_license.txt
// and downloaded on or about August 6, 2015.
// Portions of this code were derived tboot published
// by Intel under the license set forth in intel_license.txt
// and downloaded on or about August 6, 2015.
// Portions of this code were derived from the crypto utility
// published by John Manferdelli under the Apache 2.0 license.
// See github.com/jlmucb/crypto.
// File: CloudProxySignEndorsementKey.cc


// Calling sequence
//   CloudProxySignEndorsementKey.exe --cloudproxy_private_key_file=file-name [IN]
//       --endorsement_info_file=file-name [IN] --signing_instructions_file=file-name [IN]
//       --signed_endorsement_cert=file-name [OUT]

using std::string;

//  This program reads the endorsement_info_file and produces a certificate
//  for the endorsement key using the cloudproxy_signing_key in accordance with
//  the signing instructions.  signing instructions contains a subset of:
//  duration, purpose, and other information to be included in the signed certificate.

#define CALLING_SEQUENCE "Calling secquence: CloudProxySignEndorsementKey.exe" \
"--cloudproxy_private_key_file=input-file-name" \
"--endorsement_info_file=file-name  --signing_instructions_file=input-file-name" \
"--signed_endorsement_cert=output-file-name\n"

void PrintOptions() {
  printf(CALLING_SEQUENCE);
}

DEFINE_string(endorsement_info_file, "", "output file");
DEFINE_string(cloudproxy_private_key_file, "", "private key file");
DEFINE_string(signing_instructions_file, "", "signing instructions file");
DEFINE_string(signed_endorsement_cert, "", "signed endorsement cert file");

#ifndef GFLAGS_NS
#define GFLAGS_NS gflags
#endif

int main(int an, char** av) {
  LocalTpm tpm;

  GFLAGS_NS::ParseCommandLineFlags(&an, &av, true);
  if (!tpm.OpenTpm("/dev/tpm0")) {
    printf("Can't open tpm\n");
    return 1;
  }

done:
  tpm.CloseTpm();
}


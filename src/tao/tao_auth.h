//  File: tao_auth.h
//  Author: Tom Roeder <tmroeder@google.com>
//
//  Description: An interface for hosted-program authorization mechanisms
//
//  Copyright (c) 2013, Google Inc.  All rights reserved.
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

#ifndef TAO_TAO_AUTH_H_
#define TAO_TAO_AUTH_H_

#include <string>

using std::string;

namespace tao {
class TaoAuth {
 public:
  virtual ~TaoAuth() {}
  virtual bool IsAuthorized(const string &program_hash) const = 0;
  virtual bool IsAuthorized(const string &program_name,
                            const string &program_hash) const = 0;
};
}  // namespace tao

#endif  // TAO_TAO_AUTH_H_

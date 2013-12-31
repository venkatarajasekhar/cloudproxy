//  File: kvm_unix_tao_channel_unittests.cc
//  Author: Tom Roeder <tmroeder@google.com>
//
//  Description: Tests the basic KvmUnixTaoChannel with a fake Tao
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

#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <stdlib.h>

#include <thread>

#include <gtest/gtest.h>
#include <glog/logging.h>
#include <keyczar/base/base64w.h>

#include "tao/fake_tao.h"
#include "tao/kvm_unix_tao_channel.h"
#include "tao/tao.h"
#include "tao/util.h"

using std::thread;

using tao::ConnectToUnixDomainSocket;
using tao::FakeTao;
using tao::KvmUnixTaoChannel;
using tao::ScopedFd;
using tao::Tao;
using tao::TaoChannel;

class KvmUnixTaoChannelTest : public ::testing::Test {
  virtual void SetUp() {
    // Get a temporary directory to use for the files.
    string dir_template("/tmp/kvm_unix_tao_test_XXXXXX");
    scoped_array<char> temp_name(new char[dir_template.size() + 1]);
    memcpy(temp_name.get(), dir_template.data(), dir_template.size() + 1);

    ASSERT_TRUE(mkdtemp(temp_name.get()));
    dir_ = temp_name.get();

    creation_socket_ = dir_ + string("/creation_socket");
    stop_socket_ = dir_ + string("/stop_socket");

    // Pass the channel a /dev/pts entry that you can talk to and pretend to be
    // the Tao communicating with it.
    
    *master_fd_ = open("/dev/ptmx", O_RDWR);
    ASSERT_NE(*master_fd_, -1) << "Could not open a new psuedo-terminal";

    // Prepare the child pts to be opened.
    ASSERT_EQ(grantpt(*master_fd_), 0) << "Could not grant permissions for pts";
    ASSERT_EQ(unlockpt(*master_fd_), 0) << "Could not unlock the pts";
    
    char *child_path = ptsname(*master_fd_);
    ASSERT_NE(child_path, nullptr) << "Could not get the name of the child pts";

    string child_pts(child_path);

    tao_channel_.reset(new KvmUnixTaoChannel(creation_socket_, stop_socket_));

    tao_.reset(new FakeTao());

    string child_hash("Fake hash");
    string params;
    ASSERT_TRUE(tao_channel_->AddChildChannel(child_hash, &params))
      << "Could not add a child to the channel";
    ASSERT_TRUE(tao_channel_->UpdateChildParams(child_hash, child_path))
      << "Could not update the channel with the new child parameters";

    // The listening thread will continue until sent a stop message.
    listener_.reset(new thread(&KvmUnixTaoChannel::Listen, tao_channel_.get(),
      tao_.get()));
  }

  virtual void TearDown() {
    ScopedFd sock(new int(-1));
    ASSERT_TRUE(ConnectToUnixDomainSocket(stop_socket_, sock.get()));

    // It doesn't matter what message we write to the stop socket. Any message
    // on this socket causes it to stop. It doesn't even read the message.
    int msg = 0;
    ssize_t bytes_written = write(*sock, &msg, sizeof(msg));
    if (bytes_written != sizeof(msg)) {
      PLOG(ERROR) << "Could not write a message to the stop socket";
      return;
    }

    if (listener_->joinable()) {
      listener_->join();
    }
  }

  ScopedFd master_fd_;
  scoped_ptr<Tao> tao_;
  scoped_ptr<KvmUnixTaoChannel> tao_channel_;
  scoped_ptr<thread> listener_;
  string dir_;
  string creation_socket_;
  string stop_socket_;
};

GTEST_API_ int main(int argc, char **argv) {
  testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}

//  File: pipe_tao_channel.cc
//  Author: Tom Roeder <tmroeder@google.com>
//
//  Description: Implementation of PipeTaoChannel for Tao
//  communication over file descriptors
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

#include <tao/pipe_tao_channel.h>
#include <tao/pipe_tao_channel_params.pb.h>
#include <tao/tao_child_channel_params.pb.h>

#include <keyczar/base/scoped_ptr.h>

#include <stdio.h>
#include <stdlib.h>
#include <sys/select.h>
#include <sys/socket.h>
#include <sys/un.h>

#include <thread>

using std::thread;

namespace tao {
PipeTaoChannel::PipeTaoChannel(const string &socket_path)
  : domain_socket_path_(socket_path) { }
PipeTaoChannel::~PipeTaoChannel() { }

bool PipeTaoChannel::AddChildChannel(const string &child_hash, string *params) {
  if (params == nullptr) {
    LOG(ERROR) << "Could not write the params to a null string";
    return false;
  }

  // check to make sure this hash isn't already instantiated with pipes
  {
    lock_guard<mutex> l(data_m_);
    auto hash_it = hash_to_descriptors_.find(child_hash);
    if (hash_it != hash_to_descriptors_.end()) {
      LOG(ERROR) << "This child has already been instantiated with a channel";
      return false;
    }
  }

  int down_pipe[2];
  if (pipe(down_pipe)) {
    LOG(ERROR) << "Could not create the down pipe for the client";
    return false;
  }

  int up_pipe[2];
  if (pipe(up_pipe)) {
    LOG(ERROR) << "Could not create the up pipe for the client";
    return false;
  }

  // the parent connect reads on the up pipe and writes on the down pipe.
  {
    lock_guard<mutex> l(data_m_);
    hash_to_descriptors_[child_hash].first = up_pipe[0];
    hash_to_descriptors_[child_hash].second = down_pipe[1];
  }

  LOG(INFO) << "Adding program with digest " << child_hash;
  LOG(INFO) << "Pipes for child: " << down_pipe[0] << ", " << up_pipe[1];
  LOG(INFO) << "Pipes for parent: " << up_pipe[0] << ", " << down_pipe[1];

  // the child reads on the down pipe and writes on the up pipe
  PipeTaoChannelParams ptcp;
  ptcp.set_readfd(down_pipe[0]);
  ptcp.set_writefd(up_pipe[1]);

  TaoChildChannelParams tccp;
  tccp.set_channel_type("PipeTaoChannel");
  string *child_params = tccp.mutable_params();
  if (!ptcp.SerializeToString(child_params)) {
    LOG(ERROR) << "Could not serialize the child params to a string";
  }

  if (!tccp.SerializeToString(params)) {
    LOG(ERROR) << "Could not serialize the params to a string";
    return false;
  }

  // Put the child fds in a data structure for later cleanup.
  {
    lock_guard<mutex> l(data_m_);
    child_descriptors_[child_hash].first = down_pipe[0];
    child_descriptors_[child_hash].second = up_pipe[1];
  }

  return true;
}

bool PipeTaoChannel::ChildCleanup(const string &child_hash) {
  {
    // Look up this hash to see if the parent has fds to clean up
    lock_guard<mutex> l(data_m_);
    auto child_it = hash_to_descriptors_.find(child_hash);
    if (child_it == hash_to_descriptors_.end()) {
      LOG(ERROR) << "No parent descriptors to clean up";
      return false;
    }

    LOG(INFO) << "Closed " << child_it->second.first << " and "
              << child_it->second.second << " in ChildCleanup";
    close(child_it->second.first);
    close(child_it->second.second);

    hash_to_descriptors_.erase(child_it);
  }

  return true;
}

bool PipeTaoChannel::ParentCleanup(const string &child_hash) {
  {
    lock_guard<mutex> l(data_m_);
    // Look up this hash to see if this child has any params to clean up.
    auto child_it = child_descriptors_.find(child_hash);
    if (child_it == child_descriptors_.end()) {
      LOG(ERROR) << "No child " << child_hash << " for parent clean up";
      return false;
    }

    LOG(INFO) << "Closed " << child_it->second.first << " and "
              << child_it->second.second << " in ParentCleanup";
    close(child_it->second.first);
    close(child_it->second.second);

    child_descriptors_.erase(child_it);
  }

  return true;
}

bool PipeTaoChannel::ReceiveMessage(google::protobuf::Message *m,
                                    const string &child_hash) const {
  // try to receive an integer
  CHECK(m) << "m was null";

  int readfd = 0;
  {
    lock_guard<mutex> l(data_m_);
    // Look up the hash to see if we have descriptors associated with it.
    auto child_it = hash_to_descriptors_.find(child_hash);
    if (child_it == hash_to_descriptors_.end()) {
      LOG(ERROR) << "Could not find any file descriptors for " << child_hash;
      return false;
    }

    readfd = child_it->second.first;
  }

  size_t len;
  ssize_t bytes_read = read(readfd, &len, sizeof(size_t));
  if (bytes_read != sizeof(size_t)) {
    LOG(ERROR) << "Could not receive a size on the channel";
    return false;
  }

  // then read this many bytes as the message
  scoped_array<char> bytes(new char[len]);
  bytes_read = read(readfd, bytes.get(), len);

  // TODO(tmroeder): add safe integer library
  if (bytes_read != static_cast<ssize_t>(len)) {
    LOG(ERROR) << "Could not read the right number of bytes from the fd";
    return false;
  }

  string serialized(bytes.get(), len);
  return m->ParseFromString(serialized);
}

bool PipeTaoChannel::SendMessage(const google::protobuf::Message &m,
                                 const string &child_hash) const {
  // send the length then the serialized message
  string serialized;
  if (!m.SerializeToString(&serialized)) {
    LOG(ERROR) << "Could not serialize the Message to a string";
    return false;
  }

  int writefd = 0;
  {
    lock_guard<mutex> l(data_m_);
    // Look up the hash to see if we have descriptors associated with it.
    auto child_it = hash_to_descriptors_.find(child_hash);
    if (child_it == hash_to_descriptors_.end()) {
      LOG(ERROR) << "Could not find any file descriptors for " << child_hash;
      return false;
    }

    writefd = child_it->second.second;
  }

  size_t len = serialized.size();
  ssize_t bytes_written = write(writefd, &len, sizeof(size_t));
  if (bytes_written != sizeof(size_t)) {
    LOG(ERROR) << "Could not write the length to the fd " << writefd;
    return false;
  }

  bytes_written = write(writefd, serialized.data(), len);
  if (bytes_written != static_cast<ssize_t>(len)) {
    LOG(ERROR) << "Could not wire the serialized message to the fd";
    return false;
  }

  return true;
}

bool PipeTaoChannel::Listen(Tao *tao) {
  // The unix domain socket is used to listen for CreateHostedProgram requests.
  int sock = socket(AF_UNIX, SOCK_DGRAM, 0);
  if (sock == -1) {
    LOG(ERROR) << "Could not create unix domain socket to listen for messages";
    return false;
  }

  struct sockaddr_un addr;
  addr.sun_family = AF_UNIX;
  if (domain_socket_path_.size() + 1 > sizeof(addr.sun_path)) {
    LOG(ERROR) << "The path " << domain_socket_path_ << " was too long to use";
    return false;
  }

  strncpy(addr.sun_path, domain_socket_path_.c_str(), sizeof(addr.sun_path));
  int len = strlen(addr.sun_path) + sizeof(addr.sun_family);
  int bind_err = bind(sock, (struct sockaddr *)&addr, len);
  if (bind_err == -1) {
    PLOG(ERROR) << "Could not bind the address " << domain_socket_path_
                << " to the socket";
  }

  LOG(INFO) << "Bound the unix socket to " << domain_socket_path_;


  while (true) {
    // set up the select operation with the current fds and the unix socket
    fd_set read_fds;
    FD_ZERO(&read_fds);
    int max = sock;
    FD_SET(sock, &read_fds);

    for (pair<const string, pair<int, int>> &descriptor : hash_to_descriptors_) {
      int d = descriptor.second.first;
      FD_SET(d, &read_fds);
      if (d > max) {
        max = d;
      }
    }

    int err = select(max + 1, &read_fds, NULL, NULL, NULL);
    if (err == -1) {
      PLOG(ERROR) << "Error in calling select";
      return false;
    }

    // Check for messages to handle
    if (FD_ISSET(sock, &read_fds)) {
      if (!HandleProgramCreation(tao, sock)) {
        LOG(ERROR) << "Could not handle the program creation request";
      }
    }

    for (pair<const string, pair<int, int>> &descriptor : hash_to_descriptors_) {
      int d = descriptor.second.first;
      const string &child_hash = descriptor.first;

      if (FD_ISSET(d, &read_fds)) {
        TaoChannelRPC rpc;
        if (!GetRPC(&rpc, child_hash)) {
          LOG(ERROR) << "Could not get an RPC";
        }

        if (!HandleRPC(*tao, child_hash, rpc)) {
          LOG(ERROR) << "Could not handle the RPC";
        }
      }
    }
  }

  return true;
}

bool PipeTaoChannel::HandleProgramCreation(Tao *tao, int sock) {
  // Try to receive a message on the socket. This message has a size prefix and
  // the data itself.
  size_t len;
  ssize_t bytes_received = recvfrom(sock, &len, sizeof(len), MSG_DONTWAIT,
                                    NULL /* src_addr */, NULL /* addrlen */);
  if (bytes_received == -1) {
    PLOG(ERROR) << "Could not receive a length on the socket";
    return false;
  }

  scoped_array<char> bytes(new char[len]);
  bytes_received = recvfrom(sock, bytes.get(), len, MSG_DONTWAIT,
                            NULL /* src_addr */, NULL /* addrlen */);
  if (bytes_received == -1) {
    PLOG(ERROR) << "Could not receive the protocol buffer bytes from the socket";
    return false;
  }

  // This message must be a TaoChannelRPC message, and it must be have the type
  // START_HOSTED_PROGRAM
  TaoChannelRPC rpc;
  string rpc_data(bytes.get(), len);
  if (!rpc.ParseFromString(rpc_data)) {
    LOG(ERROR) << "Could not parse the data as a TaoChannelRPC";
    return false;
  }

  if ((rpc.rpc() != START_HOSTED_PROGRAM) || !rpc.has_start()) {
    LOG(ERROR) << "This RPC was not START_HOSTED_PROGRAM";
    return false;
  }

  const StartHostedProgramArgs &shpa = rpc.start();
  list<string> args;
  for(int i = 0; i < shpa.args_size(); i++) {
    args.push_back(shpa.args(i));
  }

  return tao->StartHostedProgram(shpa.path(), args);
}
}  // namespace tao

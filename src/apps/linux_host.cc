//  File: linux_tao.cc
//  Author: Tom Roeder <tmroeder@google.com>
//
//  Description: A Tao host for Linux that creates child processes.
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
#include <cstdio>
#include <list>
#include <utility>
#include <string>

#include <gflags/gflags.h>
#include <glog/logging.h>

#include "tao/linux_admin_rpc.h"
#include "tao/linux_host.h"
#include "tao/tao.h"
#include "tao/tao_domain.h"
#include "tao/util.h"

using std::list;
using std::pair;
using std::string;

using tao::InitializeApp;
using tao::LinuxAdminRPC;
using tao::LinuxHost;
using tao::Tao;
using tao::TaoDomain;
using tao::elideString;

static constexpr auto defaultHost = "TPMTao(\"file:tpm/aikblob\", \"17, 18\")";

DEFINE_string(config_path, "tao.config", "Location of tao domain configuration");
DEFINE_string(host_path, "linux_tao_host", "Location of linux host configuration");
DEFINE_string(tao_host, "",
              "Parameters to connect to host Tao. If empty try env "
              " or use a TPM default instead.");

DEFINE_bool(service, false, "Start the LinuxHost service.");
DEFINE_bool(shutdown, false, "Shut down the LinuxHost service.");

DEFINE_bool(run, false, "Start a hosted program (path and args follow --).");
DEFINE_bool(list, false, "List hosted programs.");
DEFINE_bool(stop, false, "Stop a hosted program (names follow --) .");
DEFINE_bool(kill, false, "Kill a hosted program (names follow --) .");

int main(int argc, char **argv) {
  string usage = "Administrative utility for LinuxHost.\nUsage:\n";
  string tab = "  ";
  usage += tab + argv[0] + " [options] --service\n";
  usage += tab + argv[0] + " [options] --shutdown\n";
  usage += tab + argv[0] + " [options] --run -- program args...\n";
  usage += tab + argv[0] + " [options] --stop -- name...\n";
  usage += tab + argv[0] + " [options] --kill -- name...\n";
  usage += tab + argv[0] + " [options] --list";
  google::SetUsageMessage(usage);
  InitializeApp(&argc, &argv, true);

  int ncmds = FLAGS_service + FLAGS_shutdown + FLAGS_run + FLAGS_list +
              FLAGS_kill + FLAGS_stop;
  if (ncmds > 1) {
    fprintf(stderr, "Error: Specify at most one of:\n"
                    "  --service --shutdown --run --stop --kill --list\n");
    return 1;
  }

  if (!FLAGS_tao_host.empty()) {
    setenv(Tao::HostTaoEnvVar, FLAGS_tao_host.c_str(), 1 /* overwrite */);
  } else {
    setenv(Tao::HostTaoEnvVar, defaultHost, 0 /* no overwrite */);
  }

  if (FLAGS_service) {
    Tao *tao = Tao::GetHostTao();
    CHECK(tao != nullptr) << "Could not connect to host Tao";

    scoped_ptr<TaoDomain> admin(TaoDomain::Load(FLAGS_config_path));
    CHECK(admin.get() != nullptr) << "Could not load configuration";

    scoped_ptr<LinuxHost> host(new LinuxHost(tao, admin.release(), FLAGS_host_path));
    CHECK(host->Init());

    printf("LinuxHost Service: %s\n", elideString(host->DebugString()).c_str());
    printf("Linux Tao Service started and waiting for requests\n");;

    CHECK(host->Listen());
  } else {
    scoped_ptr<LinuxAdminRPC> host(LinuxHost::Connect(FLAGS_host_path));
    CHECK(host.get() != nullptr);

    string name;
    CHECK(host->GetTaoHostName(&name));
    
    if (FLAGS_shutdown) {
      CHECK(host->Shutdown());
      printf("Shutdown: %s\n", elideString(name).c_str());
    } else if (FLAGS_run) {
      if (argc < 2) {
        fprintf(stderr, "No path or arguments given");
        return 1;
      }
      string prog = argv[1];
      list<string> args;
      for (int i = 2; i < argc; i++) {
        args.push_back(string(argv[i]));
      }

      string child_name;
      CHECK(host->StartHostedProgram(prog, args, &child_name));

      printf("Started: %s\n", child_name.c_str());
      printf("LinuxHost: %s\n", elideString(name).c_str());
    } else if (FLAGS_kill || FLAGS_stop) {
      if (argc < 2) {
        fprintf(stderr, "No names given");
        return 1;
      }
      for (int i = 1; i < argc; i++) {
        if (FLAGS_kill) {
          CHECK(host->KillHostedProgram(argv[i]));
          printf("Killed: %s\n", argv[i]);
        } else {
          CHECK(host->StopHostedProgram(argv[i]));
          printf("Requested stop: %s\n", argv[i]);
        }
      }
    } else if (FLAGS_list) {
      list<pair<string, int>> child_info;
      CHECK(host->ListHostedPrograms(&child_info));
      printf("LinuxHost: %s\n", elideString(name).c_str());
      printf("Hosts %lu programs:", child_info.size());
      for (auto &info : child_info) {
        printf(" PID %d subprin ::%s\n", info.second, info.first.c_str());
      }
    } else {
      printf("LinuxHost: %s\n", name.c_str()); // show full name here
    }
  }

  return 0;
}

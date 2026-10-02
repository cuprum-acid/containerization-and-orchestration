#!/usr/bin/env python3
import errno
import json
import os
import sys

import seccomp


def action(name, errno_ret):
    return {
        "SCMP_ACT_ALLOW": seccomp.ALLOW,
        "SCMP_ACT_ERRNO": seccomp.ERRNO(errno_ret),
        "SCMP_ACT_KILL_PROCESS": seccomp.KILL_PROCESS,
        "SCMP_ACT_LOG": seccomp.LOG,
    }[name]


def main():
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    with open(sys.argv[1]) as f:
        profile = json.load(f)

    filt = seccomp.SyscallFilter(
        defaction=action(profile["defaultAction"], profile.get("defaultErrnoRet", errno.EPERM)))
    for rule in profile.get("syscalls", []):
        act = action(rule["action"], rule.get("errnoRet", errno.EPERM))
        for name in rule["names"]:
            filt.add_rule(act, name)

    filt.load()
    os.execvp(sys.argv[2], sys.argv[2:])


main()

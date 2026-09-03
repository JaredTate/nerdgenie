// Package sandbox runs a command for the agent, either inside a fence of bwrap,
// Landlock, and seccomp, or straight on the machine as the user.
//
// There are two runners here and the configuration picks between them. NewDirect
// is the one a fresh install uses, because the sandbox setting is off unless the
// file asks for the fence: it runs the command on the machine as the user, the
// way most agents run on the host, keeping the process group, the
// timeout, the cancel, and the output cap that the fence keeps and losing only
// the box. New builds the fence, which is what the setting "fence" asks for and
// what the rest of this comment is about.
//
// The fence has two halves. Outside, this package builds a bwrap command line
// that gives the command a new user, process, message-queue, and hostname
// namespace, a fresh /proc, /dev, and a /tmp of a fixed size, the system folders
// bound read-only, the file the machine's resolver settings really live in bound
// read-only when /etc/resolv.conf is a link out of /etc, as it is on a machine
// running systemd-resolved, and the configured sandbox roots bound read-write at
// their own paths. The command itself runs under a bound on how many processes
// it may start and how much address space it may map, so that one line of shell
// cannot take the machine down. The command also gets a network namespace of its own, empty, so that it reaches
// neither the internet nor any service on this machine, unless the caller sets
// Settings.Network, which the shell tool does because a build and a package
// install both fetch what they need. Nothing else is bound, so the agent's own
// home folder, the vault, the browser profile, and the user's SSH keys are not
// there to be read.
//
// A root is held as the folder its links lead to. bwrap binds the folder a link
// leads to, and Landlock hangs its rule on the same folder, so a fence built
// from the link itself would bind one path and be asked to write under another.
// The paths that must stay outside are worked out from the user's home directory
// and from the agent's own home folder, which COEUS_HOME may have moved anywhere,
// so whoever builds a fence passes both.
//
// Inside, bwrap cannot apply Landlock, so it starts the coeus binary again as a
// hidden subcommand, sandbox-entry. That helper locks its operating-system
// thread, sets the no-new-privileges flag, builds a Landlock ruleset that allows
// reading and running the system folders and full access to the roots, installs
// a seccomp filter that answers a short list of system calls no tool needs with
// "operation not permitted", looks the program up on the PATH the fence set,
// because the system call that becomes another program searches no path of its
// own, and then becomes the command it was asked to run. Both restrictions
// survive that change of program, so the command starts already fenced in.
//
// Every run has bounds: the number of roots, the number of arguments, the number
// of environment entries, the processes the command may start, the address space
// it may map, the size of its temporary folder, the time it may take, and the
// bytes kept from each of its two output streams. An environment entry a caller
// adds may not be named PATH, HOME, or anything beginning with LD_, because the
// command is given whichever value was written last and those are the fence's
// own.
//
// Two pieces here were not asked for by brief 2.3 and are kept on purpose. The
// first is the user-namespace probe in Available: Ubuntu ships with AppArmor
// refusing an unconfined program a new user namespace, so an installed bwrap is
// not the same as a working one, and a shell tool that turns itself off on the
// strength of "bwrap is on the PATH" would be wrong on most fresh machines. The
// probe is one empty fence, asked once per fence and remembered. The second is
// Settings.HelperProgram: the fence has to start the coeus binary from inside
// itself, and a test needs to point that at a program that is really on disk,
// which the test binary is and an unbuilt release is not. It defaults to this
// program, which is what production uses.
package sandbox

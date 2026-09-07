// Package shell is the shell tool: it runs a command inside the sandbox, hands
// back an id when the command outlives ten seconds, and asks before it runs
// anything with administrator powers.
//
// Three ideas make this tool worth its weight. The first is the fence: every
// ordinary command runs through the sandbox, and when the sandbox says it cannot
// run, the tool turns itself off and says why, rather than quietly running the
// command on the bare machine. The second is the yield: a command that has not
// finished in ten seconds is not waited on, because a model that has been made
// to wait ten minutes for a build has forgotten what it was building; instead
// the tool hands back an id, keeps the command in a bounded table, and answers
// poll, tail, and kill on that id. The third is escalation: a command may carry
// a written reason and ask for administrator powers, which puts the whole
// command in front of the user as a preview, and only once that is approved does
// it run outside the fence through sudo, which reads the password from a helper
// program rather than from anything the model can see; a ruling that is not
// exactly allow runs nothing, whatever else it says.
//
// Every command goes through bash with pipefail set wherever bash is on the
// machine, so that a failing command in a pipe reports its own code rather than
// the code of the last command in the pipe, and through /bin/sh where there is
// no bash.
//
// Two more things came from watching a model work: a serve, which starts a
// command that is meant to keep running and answers the moment it listens on a
// port, because a server never finishes and a model polling one asks for ever;
// and a check, which says whether a port answers and how fast with no shell in
// it. A command that writes the same thing as the time before is answered in
// full like any other: a short answer for it used to stand here, and it broke
// the harness's own syntax check, which reads the tool's first line.
package shell

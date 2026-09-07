// Package tool is the tool registry and the twenty built-in tools, one folder
// each.
//
// A tool has five parts: a name, a description under forty words that says when
// to use it and when not to, a list of typed input fields, a permission class,
// and a function that runs it and returns text. The registry holds the whole set
// one turn can see. It builds the twenty built-in tools from one settings
// struct, then adds every executable the user dropped into the tools folder
// under the agent's home, asking each one what it is through the user-tool
// protocol in internal/contract. A description over the forty-word cap is
// refused by name, because the model reads every description on every call, and
// a user tool whose description will not parse is skipped with one logged line
// naming the file and the problem rather than stopping the agent.
//
// Every result the registry hands back is data and never instructions: the
// registry never reads the text it returns, and the working context wraps it.
// Text past the output cap in the configuration is written whole to a file under
// the run folder and the result ends with one line naming that file, so the
// model can read the rest with the read tool. Everything here is bounded: the
// number of tools, the length of a description, the size of one result, and the
// size of the spill folder, whose oldest files are removed once it is full.
package tool

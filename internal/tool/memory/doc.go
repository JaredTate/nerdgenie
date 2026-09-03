// Package memory is the memory tool: it searches what the agent knows, brings
// one fact back by its name, and writes a new one down.
//
// It is thin on purpose. Everything about how memory is kept, indexed, and
// capped belongs to the memory package behind the contract; this tool only
// reads the model's three words, calls the one method each of them means, and
// writes the answer out as lines a model can read. A fact saved here carries the
// source it came from, because a fact with no source is a rumour.
package memory

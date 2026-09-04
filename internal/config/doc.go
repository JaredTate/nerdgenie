// Package config reads and checks ~/.nerdgenie/config.toml and says where every
// file in the home folder lives.
//
// The configuration is read once, at startup. Loading starts from the defaults
// in contract.DefaultConfig, decodes the file over them, fills in the few
// defaults that depend on where the home folder is, and then checks every field,
// so an empty file and a missing file are both valid configurations. Decoding is
// strict: a key the configuration does not have is an error rather than
// something silently ignored, because a misspelled option that does nothing is
// worse than one that complains. Every error names the key, the line it is on
// when the file gives one, and what to do about it.
//
// The keys in the file are the toml tags on contract.Config, which are the field
// names written in lower case with underscores between the words, so DefaultModel
// is "default_model" and BaseAddress is "base_address"; the caps live in a [caps]
// table and the memory caps in a [memory_caps] one. A key run together without
// the underscores, such as "defaultmodel", is refused as unknown rather than
// quietly accepted. The model aliases are a table array, one [[models]] block
// each. A length of time may be written either as a string such as "30m" or as a
// whole number of nanoseconds.
//
// The doctor looks at a home folder and reports what is there, what is missing,
// what has the wrong mode, whether the configuration loads, whether the outside
// programs Coeus needs are on the path, and whether the local model daemon
// answers its health check. It never changes anything: "nerdgenie init" and "nerdgenie
// doctor" in wave 3 both print what it found.
package config

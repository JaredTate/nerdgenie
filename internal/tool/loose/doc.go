// Package loose reads the arguments of one tool call the way a model writes
// them rather than the way a tool would like them: a field under any of the
// names it is commonly given, a number in quotes, a flag written as the word
// "true", an action in capitals, and a refusal that names the field the call is
// missing and the key the model wrote instead.
//
// Every tool that takes arguments from a model reads them through this package,
// so that one badly written field is refused by name rather than killing the
// whole call, and so that the same wrong guess gets the same answer from every
// tool. The rules here come from the wave 6 live run and the gate review of
// brief 6.7, where a model that wrote old_string for old, contents for content,
// and "12" for 12 lost a round to each.
package loose

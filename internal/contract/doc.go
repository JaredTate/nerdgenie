// Package contract holds every interface, type, and constant that two packages
// of Coeus share, and it depends on nothing but the standard library.
//
// Coeus is built in waves, and the packages of one wave are written at the same
// time by workers who never see each other's code. This package is how they
// agree. A fake in internal/testkit and the real implementation written a wave
// later are both written against the lines in this package, so the two can never
// drift apart without the compiler saying so. Nothing here does any work: there
// are interfaces, the shapes that cross them, the constants both sides must
// agree on, a handful of small pure functions that format and read an
// identifier, and the defaults the configuration starts from. If you are looking
// for behavior, it is in the package that implements one of these interfaces.
//
// The one rule for changing this package is that a change here is a change to
// every wave at once, so it belongs to the orchestrator and to a brief that says
// so, never to a worker who found it convenient.
package contract

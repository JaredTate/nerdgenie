// Package testkit holds every fake a Nerd Genie test runs against, the golden-file
// helper, and the forty-step fixture.
//
// A fake stands in for something real: a model, a channel, a browser, a clock.
// Every fake here implements the matching interface in internal/contract, so the
// fake and the real thing a later wave writes are both written against the same
// lines and cannot drift apart without the compiler saying so. Beside each fake
// is a Check function that takes the interface rather than the fake, and asserts
// the properties the contract promises; a unit test calls it on the fake and a
// live test calls it on the real thing, which is what keeps the two honest.
//
// Every fake is bounded: a scripted queue runs out with a clear error rather
// than blocking, a server has a timeout, and the fake clock never waits on the
// real one. Nothing here reaches the network except through a loopback server the
// test itself started.
package testkit

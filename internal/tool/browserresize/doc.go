// Package browserresize is the browser_resize tool, which sets the size of the
// page the browser is on and reads it again at that size.
//
// A page is checked at a phone's width, a tablet's or a wide screen's this
// way, without anyone dragging the window. Until this tool existed the model
// had no way to do that: the fresh game build's visual QA task, asked to check
// five sizes, tried window.resizeTo from a script, then reached for the desktop
// tool to drag the Chrome window by hand.
package browserresize

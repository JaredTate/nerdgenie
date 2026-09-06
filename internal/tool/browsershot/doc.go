// Package browsershot is the browser_screenshot tool, which takes a picture of
// the page the browser is on for a model that can see.
//
// The picture comes from the browser worker with the page's clickable elements
// numbered on it, is saved as the next numbered file under the home's
// screenshots folder, and rides back with the result as a PNG; the turn loop
// shows it to a model that reads pictures and tells one that cannot that the
// picture is not shown. Until this tool existed the model had no way to look at
// a canvas: the thirteenth nightly run's polish task spent thirty rounds trying
// to take this picture through Chrome's debugging port.
package browsershot

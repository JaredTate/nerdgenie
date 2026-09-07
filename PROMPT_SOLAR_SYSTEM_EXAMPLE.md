<!-- PROMPT_SOLAR_SYSTEM_EXAMPLE.md: an interactive solar system ask written as a work
     order, the shape PROMPT_TEMPLATE_GUIDE.md explains. The fourth worked example: a
     smaller visual product than the Earth, with orbital motion as the thing the tests
     must prove. The runners replace <WORK> with the folder the project goes in, such
     as /home/jared/Desktop. -->

# Solar System Viewer

## Goal

Build a complete, polished, browser-based interactive solar system from scratch: the Sun and all eight major planets in smooth orbital motion, rendered with light, glow, depth and animation that look stunning and modern, and a person can zoom, pan, click a planet, read about it, pause, resume and change the speed. It should feel smooth, beautiful, educational and engaging, and work well in a normal desktop browser window. The orbital maths and the state live in their own modules, separate from the rendering, and every rule of them is proved by an automated test before it is built. It should feel like a finished product, not a first working version.

## Where

A new, empty folder: `<WORK>/Solar System Viewerv2`. It already exists. Put all source code, tests and assets inside it. Serve the app from that folder on port 8094.

## Done when

1. Every automated test passes and none is skipped. [tests pass: npm test]
2. The page loads in the browser and shows the Sun and the planets. [shows: "Solar System" at http://127.0.0.1:8094]
3. In the Chrome window on the screen, each of these was done, seen and photographed: the view zoomed from the whole system to one planet and back, panned, every planet clicked with its label and information shown, the motion paused and resumed, the speed changed, and the view reset.
4. The Sun's glow, the planets' lighting from the Sun, the orbits' depth and the smooth motion are seen and photographed.
5. At 1440, 1024 and 768 wide nothing overlaps, nothing is clipped, the controls and labels are readable, and the console shows no errors, no failed requests and no missing assets.
6. The page exposes its state as `window.solar` (simulated time, speed, paused, the selected planet, the camera, fps), and after the QA session it reports a frame rate that stayed smooth.
7. The whole test suite is green after the last change made during browser QA and visual QA.

## Rules

These are the choices already made, so the work never has to make them.

- Tests first: write the test, watch it fail, write the code, watch it pass; the whole suite green before a task ends.
- Any framework, library or build step is fine. Three.js on WebGL is the natural choice for depth and glow; plain Canvas is fine if it looks as good. Whatever you choose, `npm test` runs the whole suite and `npm start` serves the app on port 8094.
- The Chrome window on the screen, driven with the browser tools, is how the app is opened, used, photographed and checked. That window is how you see your work.
- Keep the orbital maths and the state in pure modules the renderer only reads: each planet's position from the simulated time, the speed and pause state, the selection, the camera's target and distance. Every one of them is tested without a screen.
- Keep the planet data in one file: name, radius, orbit radius, orbital period, rotation period, colour or texture, and the facts the information panel shows. Distances and sizes may be scaled for the eye, and the scale is one value in that file.
- Keep every tunable value in one config file: the time scale, the speed range, zoom limits, camera inertia, glow strength, label sizes.
- Expose the state on the page as `window.solar`, so the browser tool can ask the page what it shows instead of reading pixels.
- Build for beauty and speed together: anti-aliasing, a texture budget, one draw per planet, only redraw what moves. Measure the frame rate before and after each graphics step.
- When a test will not pass, write the failure and its cause into the record and take another route. When the page renders black or a control does nothing, read the console, fix it, and open it again. The job is finished when every done line is met.

## Tasks

1. Scaffold: `package.json`, a test runner, `index.html`, the renderer dependency, one smoke test, `npm test` and `npm start` working on 8094. Done when the smoke test passes and the page is served. (Details: Engineering)
2. The planet data and the orbital maths: the data file for the Sun and the eight planets, and a module that gives each planet's position and rotation from the simulated time, with the display scale. Done when the maths tests pass. (Details: The system, Tests required)
3. The state: simulated time advancing with a speed, pause and resume, the speed range, the selected planet, reset. Done when the state tests pass. (Details: Interaction, Tests required)
4. The camera's state: zoom between limits with inertia, pan, follow a selected planet, reset, and the mapping from a click to a planet. Done when the camera and interaction tests pass. (Details: Interaction, Tests required)
5. The system on screen: the Sun, the planets in their orbits, orbit lines with depth, a star field, anti-aliased rendering, smooth motion driven by the state. Done when it is seen in Chrome and the frame rate is measured. (Details: The system, Rendering)
6. Light and glow: the Sun's glow, the planets lit from the Sun with a night side, rings for Saturn, atmosphere or bloom where it looks good. Done when each is seen and photographed. (Details: Rendering)
7. The controls and the information: pause and resume, a speed control, reset, labels on the planets, and an information panel for the selected planet that never obscures the system. Done when the UI state tests pass and every control is used in Chrome. (Details: Interaction)
8. Browser QA: zoom from the whole system to each planet, pan, click every planet, read every label, change the speed, pause and resume, watch for stutter, resize the browser, three sizes, watch the console. Fix what it shows. Done when done lines 3 and 5 are met. (Details: Browser QA)
9. Visual QA and polish: glow, lighting, depth, motion, transitions, label placement, control styling, the star field. Anything unfinished, awkward or cheap is improved. Done when done line 4 is met and the page looks like a product. (Details: Rendering, Browser QA)
10. The performance pass and the final regression: profile, remove unnecessary rendering, redraw only what moves; then the whole suite after every fix, one more session in Chrome across the controls, the console checked. Done when done lines 6 and 7 are met. (Details: Performance)

## Details

### The system

The Sun and all eight major planets, Mercury to Neptune, each in smooth orbital motion at a period true to the real one under the display scale, each rotating, Saturn with rings, the Earth with its Moon if it looks good. Sizes and distances scaled so the whole system fits the view and a planet is still worth zooming to. Labels on every body and useful planet information: a fact or two, the orbital period, the distance from the Sun, the size.

### Interaction

Zoom smoothly between the whole system and a single planet; pan; click a planet to select it, follow it and read about it; pause and resume the motion; adjust the speed; reset the view; explore the system. Controls that are easy to find and never hide the planets.

### Rendering

Visually impressive rendering, lighting, glow, depth and animation: the Sun as a light source with a glow, the planets lit with a day side and a night side, orbit lines with depth, a star background, anti-aliasing, smooth transitions when the camera moves or a planet is selected. Modern, immersive and polished; nothing that looks like placeholder art.

### Engineering

HTML, CSS and JavaScript or TypeScript, a clean architecture with the maths, the state and the controls separated from the rendering so they can be tested deterministically. Tests run after every change and a failing test fixed before anything else is built. Responsive in a normal desktop window.

### Tests required

Orbital positions from the simulated time; rotation; the speed, pause and resume state; the selection; the camera's zoom, pan, follow and reset; the mapping from a click to a planet; the planet data's shape; UI control wiring; rendering state.

### Performance

Smooth animation: no unnecessary rendering, few draw calls, no leaks, no animation-frame problems. The frame rate read from `window.solar.fps` before and after each change.

### Browser QA

After the suite passes, open the app in the Chrome window and use it like a person: zoom, pan, click every planet, inspect every label, adjust the speed, pause and resume, and watch the motion for stutter. Then a visual pass at different sizes for layout problems, rendering problems, awkward spacing, lag, broken controls, console errors, or anything that feels unfinished. Tests passing is not proof that it looks right; looking is.

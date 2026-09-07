<!-- PROMPT_INTERACTIVE_EARTH_EXAMPLE.md: an interactive 3D Earth ask written as a
     work order, the shape PROMPT_TEMPLATE_GUIDE.md explains. The third worked example,
     beside the Tetris game and the flight simulator: a visual product whose quality
     is judged by eye as much as by test. The runners replace <WORK> with the folder
     the project goes in, such as /home/jared/Desktop. -->

# Interactive Earth

## Goal

Build a complete, visually stunning, browser-based interactive 3D Earth from scratch: the most realistic, beautiful, immersive Earth visualisation that can run inside a modern web browser, and one that stays smooth while a person spins it, zooms it and clicks on it. It looks and feels like a polished professional product, not a programming demo. The maths, the state and the controls live in their own modules, separate from the rendering, and every rule of them is proved by an automated test before it is built. The person who opens it should say: wow, this is running entirely inside a web browser?

## Where

A new, empty folder: `<WORK>/Interactive Earth`. It already exists. Put all source code, tests, assets and documentation inside it. Serve the experience from that folder on port 8093.

## Done when

1. Every automated test passes and none is skipped. [tests pass: npm test]
2. The page loads in the browser and shows the Earth. [shows: "Interactive Earth" at http://127.0.0.1:8093]
3. In the Chrome window on the screen, each of these was done, seen and photographed: the Earth rotated by dragging, zoomed from the full view to a close view and back, rotation paused and resumed, the speed changed, clouds, atmosphere, city lights and labels each toggled off and on, a location clicked and its information shown, the camera reset.
4. The day and night lighting with the city lights on the night side, the atmospheric glow, the moving clouds, the terminator line and the stars are each seen and photographed.
5. At 1440, 1024 and 390 wide nothing overlaps, nothing is clipped, the controls are readable, and the console shows no errors, no failed requests and no missing textures.
6. The page exposes its state as `window.earth` (rotation, speed, paused, each toggle, camera latitude, longitude and altitude, the selected location, fps), and after the QA session it reports a frame rate that stayed smooth.
7. The whole test suite is green after the last change made during browser QA, the performance pass and visual QA.

## Rules

These are the choices already made, so the work never has to make them.

- Tests first: write the test, watch it fail, write the code, watch it pass; the whole suite green before a task ends.
- Any framework, library or build step is fine. Three.js on WebGL is the natural choice. Whatever you choose, `npm test` runs the whole suite and `npm start` serves the page on port 8093.
- The Chrome window on the screen, driven with the browser tools, is how the Earth is opened, spun, zoomed, clicked, photographed and checked. That window is how you see your work.
- Keep the maths, the Earth's state and the camera's state in pure modules the renderer only reads: latitude and longitude to a point on the sphere and back, the sun's direction, the terminator, rotation over time, zoom limits, selection. Every one of them is tested without a screen.
- Keep every tunable value in one config file: rotation speed, zoom limits, camera inertia, atmosphere colour and thickness, cloud speed, light intensities, label sizes. Balancing is one edit.
- Expose the state on the page as `window.earth`, so the browser tool can ask the page what it shows instead of reading pixels.
- Textures: use free public Earth textures (day, night lights, clouds, a normal or specular map) copied into `assets/` when the machine can fetch them; when it cannot, generate procedural ones that still read as Earth, and say which in the documentation. The page never depends on a texture loading from the internet at run time.
- Build for beauty and speed together: anti-aliasing, a texture budget, instanced or merged geometry for lines and labels, only redraw when something moves. Measure the frame rate before and after each graphics step.
- When a test will not pass, write the failure and its cause into the record and take another route. When the page renders black or a texture is missing, read the console, fix it, and open it again. The job is finished when every done line is met.

## Tasks

1. Scaffold: `package.json`, a test runner, `index.html`, the renderer dependency, one smoke test, `npm test` and `npm start` working on 8093. Done when the smoke test passes and the page is served. (Details: Engineering)
2. The maths module: latitude and longitude to a point on the sphere and back, great-circle distance, the sun's direction from a date and time, the day and night terminator, camera position to latitude, longitude and altitude. Done when the maths tests pass. (Details: Exploration, Tests required)
3. The Earth's state: rotation over time, speed, pause and resume, each toggle, the selected location, reset. Done when the state tests pass. (Details: Interaction, Tests required)
4. The camera's state: drag to rotate with inertia, smooth zoom between limits, orbit and pan, reset, and the mapping from a click to a point on the globe. Done when the camera tests pass. (Details: Interaction, Tests required)
5. The globe on screen: the sphere, the day texture, ocean shine, anti-aliased rendering, the star field behind. Done when the Earth is seen in Chrome and the frame rate is measured. (Details: Visual requirements, Performance)
6. Light: the sun's direction, day and night blended across the terminator, the city lights on the night side, shadows, the atmospheric glow. Done when each is seen in Chrome and photographed. (Details: Visual requirements)
7. Clouds: a cloud layer above the surface that moves, casts a soft shadow where practical, and toggles. Done when it is seen moving and toggled. (Details: Visual requirements)
8. The controls on screen: pause and resume, rotation speed, the four toggles, reset, view modes if useful, laid out so nothing obscures the Earth, wired to the state module. Done when the UI state tests pass and every control is used in Chrome. (Details: Interaction)
9. Exploration: country and city labels from a small dataset in `assets/`, the latitude and longitude under the pointer, the camera position, the selected location's information, the terminator line, the equator and the major reference lines, and an optional satellite orbit. Done when the exploration tests pass and each is seen. (Details: Exploration)
10. Browser QA: rotate repeatedly, zoom from full to close, use every control, click locations, toggle every layer, resize, three viewport sizes, watch the console, look for artifacts, broken textures, clipping and overlap. Fix what it shows. Done when done lines 3 and 5 are met. (Details: Browser QA)
11. The performance pass: profile, then remove unnecessary rendering and draw calls, fix leaks and texture problems, redraw only on change. Done when done line 6 is met. (Details: Performance)
12. Visual QA and polish: lighting, atmosphere, clouds, colour, transitions, label placement, control styling. Anything mediocre, unfinished, awkward or cheap is improved. Done when done line 4 is met and the page looks like a product. (Details: Visual requirements, Browser QA)
13. Final regression: the whole suite after every fix, one more session in Chrome across the controls and toggles, the console checked. Done when done line 7 is met.

## Details

### Visual requirements

A realistic spherical Earth with detailed continents and oceans and high-resolution textures where practical; a realistic atmospheric glow; day and night lighting with visible city lights at night; clouds above the surface that move; the sun's direction and realistic shadows; a space background with stars; smooth anti-aliased rendering; beautiful lighting and colour; smooth transitions and animations. More effects where they impress without hurting performance.

### Interaction

Click and drag to rotate the Earth; zoom smoothly in and out; pan and orbit the camera; select locations or regions; reset the camera; pause and resume the rotation; adjust the rotation speed; toggle clouds, the atmosphere, the city lights and the labels; switch viewing modes if useful. Informative controls that never obscure the Earth.

### Exploration

Where practical: country and city labels; latitude and longitude; the current camera position; the selected location's information; the day and night terminator; the equator and the major reference lines; an optional satellite or orbit visualisation. Graphical quality and smooth interaction come before more features.

### Engineering

A clean, maintainable architecture with the maths, the state and the controls separated from the rendering so they can be tested deterministically. Tests run after every change and a failing test fixed before anything else is built. Documentation in the folder: how to run, how to test, the controls, where the textures came from, and how the parts fit.

### Tests required

Earth state; camera controls; coordinate calculations; rotation; UI controls; toggles; data handling for the labels; the mathematical calculations (sun direction, terminator, great circles); rendering state; interaction behaviour (drag, zoom, click to select).

### Performance

Optimise so the animation stays smooth: check for unnecessary rendering, excessive draw calls, memory leaks, texture problems, animation-frame issues, inefficient geometry and JavaScript performance problems. High graphical quality at a smooth frame rate, read from `window.earth.fps` before and after each change.

### Browser QA

After the suite passes, open the page in the Chrome window and use it like a person: rotate it repeatedly, zoom from a full-Earth view to close views, test every button and control, click locations, toggle the graphical layers, resize the browser, try three viewport sizes, inspect the console, look for rendering artifacts, broken textures, clipping and overlapping controls, check animation smoothness, check the lighting and the atmosphere, and ask whether it actually looks impressive. Tests passing is not proof that it looks right; looking is.

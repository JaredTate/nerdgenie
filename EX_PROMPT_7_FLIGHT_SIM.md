<!-- EX_PROMPT_7_FLIGHT_SIM.md: a browser flight simulator ask written as a work
     order, the shape PROMPT_TEMPLATE_GUIDE.md explains. The second worked example
     beside EX_PROMPT_3_TETRIS.md: a larger build with physics, 3D graphics and
     performance work.
     Difficulty 9 of 10. Expected: five to twelve hours, thirteen tasks; the physics and the world are the hardest work in the set, and the time depends on the GPU and the model. -->

# Flight Simulator

## Goal

Build a complete, playable, browser-based 3D flight simulator demo from scratch: the most realistic, graphically impressive, immersive flight simulator that can run inside a modern web browser while staying smooth. It is an actual flight experience, not an aircraft moving around a static scene: the player starts on the runway, takes off, flies a large world, stalls and recovers, approaches the airport, lands, or crashes and resets. The flight physics live in their own module, separate from the rendering, and every rule of them is proved by an automated test before it is built. The finished simulator looks and feels like a polished game, and the person who flies it should think: I can't believe this is running entirely inside a web browser.

## Where

Create a new folder on the Desktop named exactly `Flight Simulator`, so the project lives at `~/Desktop/Flight Simulator`. Make it if it is not there. Put all source code, tests, assets and documentation inside it, and nothing anywhere else. Serve the simulator from that folder on port 8092.

## Done when

1. Every automated test passes and none is skipped. [tests pass: npm test]
2. The simulator loads in the browser and shows the aircraft on the runway. [shows: "Flight Simulator" at http://127.0.0.1:8092]
3. A full flight has been flown in the Chrome window on the screen and each of these was seen and photographed: the take-off roll, the climb, a left turn and a right turn, a stall and the recovery, a crash into terrain and the reset, an airport approach, and a landing.
4. Every camera view works, cockpit, chase, external and orbit, with smooth transitions, seen and photographed.
5. The instruments show airspeed, altitude, heading, vertical speed, the artificial horizon, throttle, engine status, flaps, landing gear and the compass, readable at 1440 and at 1024 wide without covering the view.
6. The page exposes its state as `window.sim` (airspeed, altitude, heading, pitch, roll, throttle, gear, flaps, state, fps), and after a full flight it reports a frame rate that stayed smooth and the console shows no errors and no failed requests.
7. The whole test suite is green after the last change made during play testing, the performance pass and visual QA.

## Rules

These are the choices already made, so the work never has to make them.

- Tests first: write the test, watch it fail, write the code, watch it pass; the whole suite green before a task ends.
- Any framework, library or build step is fine. Three.js on WebGL is the natural choice. Whatever you choose, `npm test` runs the whole suite and `npm start` serves the simulator on port 8092.
- The Chrome window on the screen, driven with the browser tools, is how the simulator is opened, flown, photographed and checked. That window is how you see your work.
- Keep the flight physics in a pure module with a fixed time step, `step(state, controls, dt)`, that never touches the renderer, so every physics test is deterministic and the renderer only draws what the physics says.
- Keep every tunable value in one config file: mass, wing area, lift and drag coefficients, stall angle, thrust, gravity, control sensitivities, camera distances, fog and lighting values. Balancing is one edit.
- Expose the simulator's state on the page as `window.sim`, so the browser tool can ask the page questions while flying instead of reading pixels.
- Build the world for size and speed together: procedural terrain, instanced trees and buildings, level of detail, culling, object pooling, and shaders where they pay for themselves. Measure the frame rate before and after each graphics step.
- When a test will not pass, write the failure and its cause into the record and take another route. When the page will not open or renders black, read the console, fix it, and open it again. The job is finished when every done line is met.

## Tasks

1. Scaffold: `package.json`, a test runner, `index.html`, the renderer dependency, one smoke test, `npm test` and `npm start` working on 8092. Done when the smoke test passes and the page is served. (Details: Engineering)
2. The physics core: aircraft state, throttle and thrust, lift, drag, gravity, velocity and inertia, altitude, heading, pitch, roll and yaw integrated with a fixed step. Done when the physics tests pass. (Details: Aircraft, Tests required)
3. Stall, ground contact, landing, crash and reset: stall conditions and recovery, ground collision, a landing that counts and a crash that counts, reset to the runway. Done when their tests pass. (Details: Aircraft, Gameplay, Tests required)
4. Controls: keyboard and mouse mapped to smoothed control inputs, throttle, pitch, roll, rudder, flaps, gear, brakes, camera keys, reset. Done when the controls and UI state tests pass. (Details: Controls, Tests required)
5. The world: terrain with mountains, valleys, fields and water, roads, an airport with a runway and markings, buildings, instanced trees, level of detail and culling. Done when the scene renders in Chrome with the runway and terrain visible and the frame rate is measured. (Details: World, Performance)
6. Sky and atmosphere: sky gradient, sun, directional lighting, shadows, distance fog and haze, horizon, clouds. Done when each is seen in Chrome and the frame rate is still smooth. (Details: Graphics)
7. The aircraft on screen: the model and its materials, cockpit glass, landing lights, runway lights, gear, flaps and control surfaces animated, water reflections where practical. Done when the aircraft is seen from outside and from the cockpit. (Details: Graphics, Cameras)
8. Cameras: cockpit, chase, external and orbit, with smooth transitions and a camera state module. Done when the camera tests pass and each view is seen. (Details: Cameras, Tests required)
9. Instruments and HUD: airspeed, altitude, heading, vertical speed, artificial horizon, throttle, engine status, flaps, gear, compass, readable and unobtrusive. Done when the UI state tests pass and done line 5 is met. (Details: Instruments and HUD)
10. Fly it in Chrome: take off, climb, turn both ways, aggressive manoeuvres, stall and recover, fly into terrain, approach, several landings, every camera, every control, resize the window, watch the console. Fix what the flight shows. Done when done line 3 is met. (Details: Browser play testing)
11. The performance pass: profile, then instancing, level of detail, pooling and culling where the profile says; fix physics instability and animation-frame problems. Done when done line 6 is met. (Details: Performance)
12. Visual QA and polish: the aircraft, cockpit, terrain, horizon, sky, clouds, lighting, shadows, runway, water, HUD, transitions, animations and readability, at 1440 and 1024 wide. Done when nothing looks placeholder or unfinished and done line 5 is met. (Details: Visual QA)
13. Final regression: the whole suite after every fix, one more flight with a take-off, a stall, a landing and every camera, the console checked. Done when done line 7 is met.

## Details

### Aircraft

A controllable aircraft with believable flight behaviour: pitch, roll, yaw, throttle, lift, drag, gravity, airspeed, altitude, stall behaviour, ground collision, basic landing behaviour, and inertia and momentum. The physics need not match a certified simulator, but they must feel believable, consistent, and substantially more realistic than arcade movement: lift grows with airspeed and angle of attack up to the stall angle and collapses past it, drag grows with speed and with flaps and gear, thrust follows the throttle with lag, and the aircraft keeps its momentum through turns.

### Controls

Keyboard controls, and mouse controls where practical: throttle up and down, pitch up and down, roll left and right, rudder, flaps, landing gear, brakes, camera controls, reset. Inputs are smoothed so control feels responsive without being jerky or over-sensitive.

### World

A large, visually impressive environment with as many of these as practical: a large terrain area, mountains, valleys, fields, water, roads, trees or vegetation, an airport, a runway with markings, buildings, clouds, atmospheric haze, a realistic sky, the sun, directional lighting, shadows, distance fog, horizon effects. Use procedural generation, efficient geometry, textures, shaders, instancing and level of detail. The world must feel large; the aircraft must never feel like it is flying over a tiny game board.

### Graphics

Graphical realism is a major priority: physically convincing lighting, a realistic sky gradient, an approximation of atmospheric scattering, sunlight, shadows, reflections, water effects, clouds, terrain variation, distance haze, motion effects, cockpit glass, aircraft materials, landing lights, runway lights, smooth animations. Balance visual quality against browser performance; a smooth frame rate wins over one more effect.

### Cameras

Cockpit view with a believable forward view and the instruments, chase camera, external aircraft view, and a free or orbit camera. Transitions are smooth.

### Instruments and HUD

Airspeed, altitude, heading, vertical speed, artificial horizon, throttle, engine status, flaps, landing gear, compass. Polished and readable without covering too much of the screen.

### Gameplay

The player can start on or near the runway, increase throttle, accelerate, take off, fly around the world, climb, descend, turn, stall when flown badly, recover from a reasonable stall, approach the airport, land, and crash when terrain is hit too hard. After a crash, reset.

### Engineering

Clean, modular, maintainable code. The physics separated from the rendering so they can be tested deterministically. The test suite run after every change; a failing test fixed before anything else is built. Documentation in the folder: how to run, how to test, the controls, and how the parts fit.

### Tests required

Aircraft state; throttle; acceleration; lift; drag; gravity; velocity; altitude; heading; pitch; roll; yaw; stall conditions and recovery; collision detection; landing state; crash state; reset; camera state; controls and input smoothing; UI and instrument state.

### Performance

Profile and optimise. Check for low frame rates, unnecessary draw calls, excessive polygons, expensive shaders, memory leaks, excessive object creation, inefficient terrain, poorly optimised vegetation, physics instability, animation-frame problems. Use instancing, level of detail, object pooling, culling and efficient rendering where they help. Aim for a smooth, responsive experience in Chrome, and read the frame rate from `window.sim.fps` before and after each change.

### Browser play testing

Only after the whole suite passes, open the simulator in the Chrome window and fly it like a person. Take off from the runway, climb, turn left and right, test pitch, roll and yaw, test the throttle response, attempt aggressive manoeuvres, stall the aircraft on purpose and recover, fly toward terrain and check the crash, fly an airport approach, attempt several landings, test every camera view and every control, resize the browser, inspect the console, and watch smoothness. Checking that buttons respond is not a test; actually fly it.

### Visual QA

A dedicated pass, as a pilot would see it: the aircraft, the cockpit, terrain, horizon, sky, clouds, lighting, shadows, runway, airport, water, HUD, instruments, camera transitions, animations, UI spacing, readability, responsiveness, visual artifacts. Ask whether it looks convincing while flying. Anything that looks cheap, placeholder, broken, distracting or unfinished is improved.

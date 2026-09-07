<!-- EX_PROMPT_5_TOWER_DEFENSE.md: a tower-defense game ask written as a work order, the
     shape PROMPT_TEMPLATE_GUIDE.md explains. The fifth worked example: the largest
     game so far, with pathfinding, an economy and balance that only play can prove.
     The runners replace <WORK> with the folder the project goes in, such as
     /home/jared/Desktop.
     Difficulty 8 of 10. Expected: four to ten hours, thirteen tasks; the balance play is most of it, and the time depends on the GPU and the model. -->

# Tower Defense Game

## Goal

Build a complete, polished, browser-based tower-defense game from scratch: a beautiful, modern game that runs entirely in the browser and feels like a real finished game, not a coding demo. Enemies walk a path from a spawn to the player's base; the player places and upgrades towers of clearly different kinds; towers find targets and fire; enemies die and pay; waves grow harder with variety, not just more health; the player loses at zero health and wins after the final wave. The game's logic lives in its own modules, separate from the rendering, deterministic, and every rule of it is proved by an automated test before it is built. Balance is proved by playing whole games. The person who plays it should think: this feels like an actual tower-defense game.

## Where

A new, empty folder: `<WORK>/Tower Defense Game`. It already exists. Put all source code, tests, assets and documentation inside it. Serve the game from that folder on port 8095.

## Done when

1. Every automated test passes and none is skipped. [tests pass: npm test]
2. The game loads in the browser and shows the map with the path, the spawn and the base. [shows: "Tower Defense" at http://127.0.0.1:8095]
3. Whole games have been played in the Chrome window on the screen and each of these was seen and photographed: every tower type placed, an invalid placement refused, a tower upgraded, a tower sold, each targeting mode chosen, enemies reaching the base, pause and resume, restart, a boss wave, a game over, a victory.
4. Balance was checked by play: the first waves are easy, no single tower wins every game, upgrades matter, money forces choices, later waves need strategy, the boss is hard but beatable, and the game cannot be made unwinnable by its own economy. The values changed by that play are in the config file.
5. At 1440, 1024 and 768 wide nothing overlaps, nothing is clipped, every number on the screen is readable, and the console shows no errors and no failed requests.
6. The page exposes its state as `window.game` (wave, health, money, enemies alive, towers, projectiles, paused, state, fps), and during the largest wave it reports a frame rate that stayed smooth.
7. The whole test suite is green after the last change made during play testing, balance testing, the performance pass and visual QA.

## Rules

These are the choices already made, so the work never has to make them.

- Tests first: write the test, watch it fail, write the code, watch it pass; the whole suite green before a task ends.
- Any framework, library or build step is fine. Canvas or WebGL as you judge; Three.js if you want depth. Whatever you choose, `npm test` runs the whole suite and `npm start` serves the game on port 8095.
- The Chrome window on the screen, driven with the browser tools, is how the game is opened, played, photographed and checked. That window is how you see your work.
- Keep the game logic in pure modules the renderer only reads, one per system: game state, map, pathfinding, towers, enemies, projectiles, waves, economy, targeting, input. Step them with a fixed time step and an injectable random source, so every test is deterministic and a whole game can be simulated without a screen.
- Keep every tower, enemy and wave definition and every balance value in one config file: costs, damage, fire rates, ranges, speeds, health, armour, rewards, upgrade costs and effects, sell value, starting money, wave composition. Balancing is one edit and the balance tests read the same file.
- Pathfinding is a real algorithm, A star or a flow field, over a grid; a placement that would leave the enemies no route is refused before it lands, and a procedural map is checked for a route before it is used.
- Expose the state on the page as `window.game`, so the browser tool can ask the page what is happening instead of reading pixels.
- Build for satisfaction and speed together: pooled projectiles and particles, a spatial grid for target searches, redraw only what moves. Measure the frame rate during the largest wave before and after each graphics step.
- When a test will not pass, write the failure and its cause into the record and take another route. When the page renders black or a control does nothing, read the console, fix it, and open it again. The job is finished when every done line is met.

## Tasks

1. Scaffold: `package.json`, a test runner, `index.html`, one smoke test, `npm test` and `npm start` working on 8095. Done when the smoke test passes and the page is served. (Details: Engineering)
2. The map and pathfinding: the grid, the spawn, the base, walkable cells, placement zones, placement validation, the path algorithm, the rule that a placement may not block every route, and a procedural map that is always playable. Done when the map and path tests pass. (Details: Map and pathfinding, Tests required)
3. Enemies and waves: the enemy types with health, speed, armour, reward and size; movement along the path; damage and death; base damage and player health; the wave system with countdown, start, composition, growth and boss waves. Done when the enemy and wave tests pass. (Details: Enemies, Waves, Tests required)
4. Towers, targeting and projectiles: the tower types with damage, fire rate, range, projectile speed, cost and upgrades; the targeting modes; projectiles, splash, slow and area effects; upgrade and sell with the sell value. Done when the tower, targeting and projectile tests pass. (Details: Towers, Targeting, Tests required)
5. The economy and the game state: starting money, purchase, rewards, upgrade costs, selling, wave rewards, pause and resume, restart, game over, victory, and a whole simulated game from wave one to the end with no screen. Done when the economy and state tests pass and the simulated game ends in a victory with a reasonable strategy and a loss with none. (Details: Economy, Player state, Balance testing, Tests required)
6. The game on screen: the map and path drawn, towers and enemies drawn and animated, projectiles, health bars, range indicators, selection, hover and placement previews with invalid placement shown clearly. Done when a wave can be played with the mouse in Chrome and the console shows no errors. (Details: UI and UX, Visual effects)
7. The interface: health, money, wave, enemies remaining, score, the selected tower with its upgrade information, tower buttons with costs and tooltips, pause, resume, restart, the game-over and victory screens, a mute control. Done when the UI state tests pass and every control is used in Chrome. (Details: Player state, UI and UX)
8. Effects and sound: muzzle flashes, impacts, explosions, particles, damage flashes, death effects, tower animations, lighting and shadows, animated environment details, UI transitions, and the sounds under Audio. Done when each is seen or heard in Chrome and the board stays readable. (Details: Visual effects, Audio)
9. Play testing in Chrome: several whole games, every tower placed, valid and invalid placements, upgrades, selling, every targeting mode, enemies reaching the base, pause and resume, restart, high waves, a boss, a game over, a victory, three sizes, the console. Fix what the play shows. Done when done lines 3 and 5 are met. (Details: Browser play testing)
10. Balance testing: play several complete games and adjust the config file until done line 4 holds; keep the simulated-game test in step with the new values. Done when done line 4 is met. (Details: Balance testing)
11. The performance pass: stress the game with large waves, profile, then a spatial grid for collisions and target searches, pooling for projectiles and particles, cached paths, and cheaper rendering where the profile says. Done when done line 6 is met. (Details: Performance)
12. Visual QA and polish: the map, terrain, paths, towers, enemies, projectiles, explosions, health bars, range indicators, previews, animations, particles, typography, spacing, buttons, menus, wave indicators, the end screens. Anything cheap, confusing, broken or unfinished is improved. Done when the game looks like a product. (Details: Visual QA)
13. Final regression: the whole suite after every fix, one more game to a boss wave in Chrome, the console checked. Done when done line 7 is met.

## Details

### Core gameplay

A complete tower-defense loop: a polished playable map, procedural or made; a clearly defined enemy path from spawn to base that enemies follow correctly; towers placed in valid locations that acquire targets and fire; enemies that take damage, die and reward money; enemies reaching the base reduce the player's health; the player loses at zero health; waves grow progressively harder; the player wins after the final wave.

### Map and pathfinding

An enemy spawn point, the player's base, walkable enemy paths, tower placement zones, collision and placement validation, pathfinding by A star, Dijkstra, a flow field or another real algorithm; enemies dynamically find a valid route; tower placement can never create an impossible path; a procedural map is always playable.

### Towers

Several tower types with clearly different purposes: a rapid-fire tower, a heavy cannon, a long-range sniper, a splash-damage tower, a slow or freeze tower, an area-of-effect tower. Each with damage, fire rate, range, projectile speed, targeting behaviour, cost and upgrade cost. Towers can be selected and upgraded, and upgrades meaningfully improve damage, range, fire rate or a special ability. The range is shown when a tower is selected.

### Enemies

Several enemy types: a basic enemy, a fast one, a heavy armoured one, a high-health tank, a swarm enemy, a resistant or special enemy, a boss. Each with health, speed, armour or resistance, reward, size and special characteristics. Waves are visually and mechanically distinct.

### Waves

A wave number, a countdown before each wave, a start-wave control, increasing difficulty, different enemy combinations, larger waves over time, boss or challenge waves, reward progression. Early waves teach the mechanics; later waves require strategy. Difficulty comes from variety, never from scaling enemy health without end.

### Economy

Starting money, tower purchase costs, enemy kill rewards, tower upgrade costs, selling towers with a sell-value calculation, wave rewards where appropriate. Money is tight enough that the player must make meaningful choices.

### Player state

Shown clearly at all times: health, money, the current wave, enemies remaining, the score, the selected tower and its upgrade information. Pause, resume, restart, a game-over screen, a victory screen.

### Targeting

First enemy, last enemy, strongest, weakest, closest, chosen per tower, updated correctly as enemies move and die.

### Visual effects

Smooth projectiles, impact effects, muzzle flashes, explosions, particles, damage flashes, enemy death effects, tower animations, health bars, range indicators, selection effects, shadows, lighting, animated environment details, smooth UI transitions. Each tower's attack is visually recognisable. The screen never fills with effects to the point that play is unreadable.

### Audio

Tasteful sounds for tower firing, impacts, explosions, enemy deaths, tower placement, upgrades, wave start, victory and game over, playing only after the player has interacted with the page, with an easy mute control.

### UI and UX

A modern, polished interface. The player understands at once where enemies enter, where they are going, where towers can go, what each tower costs and does, how much money and health they have, and what wave they are on. Tooltips or concise tower descriptions. Intuitive placement with hover, selection and placement previews, and invalid placement shown clearly.

### Engineering

A clean, modular architecture with the systems separated: game state, map, pathfinding, towers, enemies, projectiles, waves, economy, rendering, UI, input. Gameplay deterministic so it can be tested; tests run after every change; failing tests fixed before anything else is built. Documentation in the folder: how to run, how to test, the controls, the tower and enemy tables, and how the parts fit.

### Tests required

Map generation; path validity; pathfinding; tower placement; invalid tower placement; tower targeting; range calculations; damage calculations; fire rate; projectile behaviour; enemy health; enemy movement; enemy death; rewards; base damage; player health; upgrade calculations; selling towers; wave spawning; wave completion; victory; game over; economy balance rules; pause and resume state; a whole simulated game.

### Balance testing

Play several complete games and check that the first waves are reasonably easy, towers feel meaningfully different, no tower dominates every strategy, upgrades are worthwhile, money progression feels reasonable, later waves become challenging, bosses are difficult but beatable, and the game cannot enter an unwinnable state through its economy. Adjust the config values from actual play.

### Browser play testing

After the suite passes, open the game in the Chrome window and play it like a person: multiple waves, every tower type placed, valid and invalid placements, upgrades, selling, every targeting mode, enemies reaching the base, pause and resume, restart, high-level waves, at least one boss wave, game over, victory, resizing, different viewport sizes, the console, and memory, rendering and performance problems. Clicking each button once is not a test; play the game and judge whether it is fun, understandable, balanced and polished.

### Visual QA

A dedicated pass over the map, terrain, paths, towers, enemies, projectiles, explosions, health bars, range indicators, placement previews, animations, particles, the UI, typography, spacing, buttons, tower selection, upgrade menus, wave indicators, the game-over and victory screens. Look for overlapping UI, clipping, broken animations, poor contrast, hard-to-read information, glitches, wrong positioning, placeholder graphics, frame-rate drops, console errors. Anything cheap, confusing, broken or unfinished is improved.

### Performance

Stress the game with large waves and many towers and projectiles at once. Optimise collision checks, target searches, pathfinding, projectile updates, particles, rendering, object allocation and enemy updates with spatial partitioning, object pooling and caching where they help. The game stays smooth during the largest battles, read from `window.game.fps`.

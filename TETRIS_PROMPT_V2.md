<!-- TETRIS_PROMPT_V2.md: the Tater Tots Tetris ask written as a work order, the
     shape PROMPT_TEMPLATE_GUIDE.md explains. It says everything TETRIS_TEST_PROMPT.md
     says, in the order the harness reads. The runners replace <WORK> with the folder
     the game goes in, such as /home/jared/Desktop. -->

# Tater Tots Tetris

## Goal

Build a complete, polished, playable Tetris-style web game called Tater Tots Tetris, for players in a browser. It plays like a good normal Tetris game, with distinctive Tater Tots Tetris branding and a board that stays easy to read. On top of normal play it has two hazards that invade the game as characters, a dragon that breathes fire and locks the falling piece, and a yeti that blows the piece sideways with polar wind, and every cleared line goes out with one of three dramatic effects: freeze and disintegrate, bomb, or fire. Every rule of the game is proved by an automated test before it is built, and the finished game is played and looked at in Chrome before it is called done. It should feel like a small indie game, not a coding demo.

## Where

A new, empty folder: `<WORK>/Tater Tots Tetrisv1`. It already exists. Put all source code, tests, assets, sound effects and supporting files inside it. Serve the game from that folder on port 8091.

## Done when

1. Every automated test passes and none is skipped. [tests pass: npm test]
2. The game loads in the browser and shows the board. [shows: "Tater Tots Tetris" at http://127.0.0.1:8091]
3. A whole game has been played in the Chrome window on the screen, to game over, and each of these was seen and photographed: normal play, the dragon warning, the dragon attack, the yeti from the left, the yeti from the right, a freeze clear, a bomb clear, a fire clear, a multi-line clear, pause, game over, restart.
4. The layout was looked at in Chrome at five window sizes (large desktop, laptop, narrow desktop, tablet, phone) with nothing overlapping, nothing clipped and nothing outside the window, and the browser console shows no errors, no failed requests and no missing assets.
5. Restart and game over clear every hazard timer, and no hazard can touch the wrong piece, proved by tests.
6. The developer controls used for testing are hidden in the finished game.
7. The whole test suite is green after the last change made during play testing and visual QA.

## Rules

- Use any framework, library or build step you like, or none. Whatever you choose, `npm test` must run the whole suite and `npm start` must serve the game on port 8091.
- The browser is the Chrome window on the screen, driven with the browser tools. Never start a browser from a script, headless or not.
- Port 8090 is taken. Use 8091.
- Every hazard value lives in one config file and is easy to change: `DRAGON_CHANCE`, `YETI_CHANCE`, `DRAGON_WARNING_TIME`, `YETI_WARNING_TIME`, `FORCED_DROP_SPEED`, `LINE_CLEAR_EFFECT_MODE`, `LINE_CLEAR_ANIMATION_DURATION`.
- The random source is injectable or seeded, so a test can force any piece, any hazard, either yeti side and any of the three clear effects. No test may depend on luck.
- Hazards are real game states in the engine, never animations layered over the game.
- Effects must never make the board hard to read.
- Stop if a test cannot be made to pass after three different fixes.
- Stop if the page cannot be opened in the Chrome window.
- Stop if the game would need anything installed beyond npm packages.

## Tasks

1. Scaffold: `package.json`, a test runner, `index.html`, one smoke test, `npm test` and `npm start` working. Done when the smoke test passes and the page is served on 8091. (Details: Core game)
2. The engine core: the board, the seven pieces, spawning, left and right movement, rotation, wall, block and bottom collision, soft drop, hard drop, piece locking. Done when the core tests for those pass. (Details: Core game, Tests required)
3. The rest of the engine: line detection and clearing, multi-line clears, scoring, levels, increasing speed, game over, restart, pause, next-piece logic, high scores kept in local storage. Done when their tests pass. (Details: Core game, Tests required)
4. The random source and the config file: an injectable random source, and one config file holding every value the rules name. Done when a test forces a chosen piece sequence and a chosen hazard. (Details: Hazard architecture, Deterministic testing)
5. The hazard state machine: the states, one hazard per piece at a time, hazard state bound to the right piece, clean cancellation when the piece locks first. Done when the state tests pass. (Details: Hazard architecture)
6. The dragon: probability, trigger, warning state with player control, attack transition, rotation and horizontal lock, forced vertical drop, collision during the drop, locking, cleanup, cancellation. Done when the dragon tests pass. (Details: Dragon, Tests required)
7. The yeti: probability, trigger, random side, wind from the left pushes right and from the right pushes left, maximum legal displacement, collision during the wind, rotation and horizontal lock, forced drop, locking, cleanup. Done when the yeti tests pass. (Details: Yeti, Tests required)
8. Line-clear effect logic: the animating state, random choice among the three effects, forced choice in tests, logical removal after the effect, row collapse, multi-line clears, scoring after the animation, clears after dragon and yeti forced drops. Done when the effect tests pass. (Details: Line clear effects, Tests required)
9. The hazard safety suite: every line under Hazard safety in the tests. Done when it passes and the whole suite is green. (Details: Hazard architecture, Tests required)
10. The browser shell: the board and pieces drawn, the HUD, keyboard controls, the start screen, pause overlay, game-over screen, instructions, high scores, and the developer controls that force each hazard and each effect. Done when the page shows the board, a game can be played with the keyboard, and the console shows no errors. (Details: Core game, Screens and HUD, Browser play testing)
11. The characters and the effects on screen: the dragon's flight, fire and impact, the yeti's entry, snowball and wind, the three clear animations, screen shake, particles, warnings and sound. Done when each can be forced with the developer controls and is seen in the Chrome window. (Details: Dragon, Yeti, Line clear effects, Sound, Visual polish)
12. Play test in Chrome: play to game over, force each hazard from each side and each effect, try the edge cases, watch the console, fix what the play shows. Done when done line 3 is met. (Details: Browser play testing)
13. Visual QA at the five sizes, then polish. Done when done line 4 is met. (Details: Visual QA, Visual polish)
14. Final regression: the whole suite after every fix, one more short play with both hazards and all three effects, the console checked, the developer controls hidden. Done when done lines 6 and 7 are met.

## Details

### Core game

The game runs in a browser with HTML, CSS and JavaScript or TypeScript. It should be modern, responsive, visually polished, smooth, fast, fun, easy to understand and professionally presented, with Tater Tots Tetris branding that keeps the board and the play easy to read.

Normal Tetris play, all of it:

- Falling pieces, the seven standard pieces, and piece spawning at the top.
- Left and right movement, soft drop, hard drop, rotation.
- Collision with the walls, the floor and the locked blocks.
- Piece locking, line detection, line clearing, multi-line clearing.
- Scoring, levels, increasing speed and difficulty as levels rise.
- Game over when a piece cannot spawn, and restart.
- Pause and resume.
- Next-piece preview.
- Keyboard controls: move, rotate, soft drop, hard drop, pause, restart.
- Score, level and lines-cleared displays.
- High scores kept in the browser's local storage and shown to the player.

Outside the hazards below, the game behaves like a polished normal Tetris game.

### Screens and HUD

- A start screen with the name, the instructions and a way to begin.
- A pause overlay.
- A game-over screen with the score, the high scores and a way to restart.
- A responsive HUD: score, level, lines, next piece, and the hazard warnings.
- Score animations when points land.
- Attractive menus. Everything readable at every size in Visual QA.

### Dragon

About one piece in ten, at random and never exactly every tenth piece, may trigger a dragon attack. The chance is `DRAGON_CHANCE` in the config file.

When it triggers:

1. A short visual and audio warning that the dragon is coming.
2. A large animated dragon flies in from above the board.
3. The player has a brief time (`DRAGON_WARNING_TIME`) to hurry and position the current piece.
4. The dragon flies over the board and breathes fire onto the active piece.
5. The fire locks the piece's horizontal position and rotation.
6. The player loses control of the piece at once.
7. The piece crashes straight down at `FORCED_DROP_SPEED` until it meets the stack or the floor.
8. It locks normally, and play continues with the next piece.

Polish: the dragon's flight animation, fire breath, flames, sparks, embers, heat effects, screen shake, a roar, warning effects, an impact animation. The player must understand at once: the dragon is coming, move fast before it locks your piece.

### Yeti

A second random hazard: an abominable snowman, the yeti, attacking from the left or the right. About ten percent by default, `YETI_CHANCE` in the config file.

When it begins:

1. A short warning (`YETI_WARNING_TIME`).
2. The side is chosen at random, left or right.
3. The yeti animates in from that side.
4. The player has a brief chance to reposition the piece.
5. The yeti throws a giant snowball.
6. The yeti unleashes a blast of polar wind across the board.

Yeti on the right: the wind blows right to left and pushes the piece as far left as the rules allow. Yeti on the left: the wind blows left to right and pushes the piece as far right as the rules allow. The player must understand at once: yeti on the left, piece blown right; yeti on the right, piece blown left.

After the wind hits: move the piece as far sideways as collision allows, lock its horizontal position and rotation, disable control, force it to fall until it collides, lock it normally, continue play. The wind respects the board and the locked blocks: never push a piece outside the board, through a block, or into an occupied cell.

Polish: an animated yeti, a giant snowball, blowing snow, snow particles, frost, ice, wind streaks, screen shake, fitting sounds.

### Line clear effects

A completed line never just vanishes. Every clear uses one of three effects, chosen at random, so the game feels varied and fun:

1. **Freeze and disintegrate.** The line freezes with frost, pauses briefly, explodes, and breaks into particles and fragments. Frost spreading, ice crystals, cold shimmer, a sharp crack, shattering, debris.
2. **Bomb.** A flash, an explosion burst, a shockwave, debris, smoke, screen shake, an impact sound.
3. **Fire.** Flames spread across the line and burn it away: a burning glow, embers, smoke, charring, ash, a burning sound.

Rules for the effects: the choice is random (and forceable in tests through `LINE_CLEAR_EFFECT_MODE`); the animation stays clear and readable; it lasts `LINE_CLEAR_ANIMATION_DURATION`; it never breaks gameplay timing; the board stays logically correct. Clears must resolve correctly for single lines, multiple lines, back-to-back clears, and clears after a dragon or yeti forced drop. When several lines clear at once, either one effect for all of them or one per line is fine, as long as it looks polished and behaves consistently. The effects should be dramatic, funny, readable and exciting, and must never obscure the board so much that play becomes confusing.

### Hazard architecture

The hazards are real game logic. The engine has explicit states: `NORMAL`, `DRAGON_WARNING`, `DRAGON_ATTACK`, `YETI_WARNING`, `YETI_ATTACK`, `FORCED_DROP`, `PIECE_LOCKED`, `LINE_CLEAR_ANIMATING`.

- Only one major hazard may affect a piece at a time. A dragon attack and a yeti attack never affect the same piece.
- Hazard state is bound to the active piece it was raised for. A delayed timer or animation must never touch the next piece.
- If the player hard-drops a piece before a hazard reaches it, the hazard is cancelled or resolved cleanly, without corrupting play.
- The config values are in one file, easy to adjust while balancing the game: `DRAGON_CHANCE`, `YETI_CHANCE`, `DRAGON_WARNING_TIME`, `YETI_WARNING_TIME`, `FORCED_DROP_SPEED`, `LINE_CLEAR_EFFECT_MODE`, `LINE_CLEAR_ANIMATION_DURATION`.

### Deterministic testing

Random systems must be testable. Use an injectable random source, a seeded one, or mockable randomness. No test may run repeatedly hoping a dragon appears. A test can force a dragon, force a yeti from either side, and force any one of the three clear effects.

### Tests required

Core Tetris: board initialisation; piece generation; piece spawning; left movement; right movement; rotation; wall collision; block collision; bottom collision; soft drop; hard drop; piece locking; line detection; line clearing; multiple-line clearing; score calculation; level progression; increasing speed; game-over detection; restart; pause; next-piece logic.

Dragon: probability logic; event triggering; the warning state; player control during the warning; the attack transition; rotation lock; horizontal lock; forced vertical drop; collision during the forced drop; locking after the forced drop; hazard cleanup; cancellation when the piece has already locked.

Yeti: probability logic; event triggering; random direction; left-side yeti pushes right; right-side yeti pushes left; maximum legal displacement; collision during the wind; rotation lock; horizontal lock; forced vertical drop; locking after the forced drop; hazard cleanup.

Line clear effects: completed-line detection; the animating state is entered; random selection among the three; each of the three can be forced; logical removal after each effect; correct row collapse after the animation; multi-line clears resolve; scoring resolves after the animation; the board is not corrupted; next-piece spawning is not disturbed; clears work after dragon forced drops and after yeti forced drops.

Hazard safety: dragon and yeti cannot trigger at the same time; a hazard cannot affect the wrong piece; a hazard cannot survive into the next piece; restart clears all hazard timers; game over clears all hazard timers; pausing does not corrupt hazard state; rapid repeated inputs do not break forced movement; timers cannot race; animation timing cannot change game logic.

### Browser play testing

Only after the whole suite passes, open the game in the Chrome window on the screen and play it like a person. Loading the page is not a test. Play enough games to exercise: movement, rotation, soft drop, hard drop, line clearing, scoring, level progression, increasing speed, pause, restart, game over, high scores, dragon attacks, yeti attacks from both sides, the warning timings, forced movement and forced drops, hazard collision behaviour, rapid keyboard input, edge-of-board behaviour, complex stacks, all three clear effects, multi-line clears with effects, and whether the effects feel satisfying and readable.

Developer controls are allowed for testing: trigger dragon, trigger yeti from the left, trigger yeti from the right, trigger a freeze clear, a bomb clear, a fire clear. Use them to test the mechanics repeatedly. They are hidden in the finished game.

### Visual QA

After play testing, a dedicated visual pass, as a player would see it, at five sizes: large desktop, standard laptop, narrow desktop window, tablet, phone. Look for broken layouts, ugly spacing, alignment problems, bad typography, poor colour contrast, board scaling problems, controls or HUD overlapping the board, clipped dragon or yeti animations, fire or snow covering important information, broken or weak clear animations, broken transitions, janky movement, low frame rate, wrong responsive behaviour, elements outside the window, inconsistent styling, pixelated visuals, bad animation timing, pieces drifting from their logical position, screen shake that is too strong, unclear hazard warnings, anything that looks unfinished. Open the console and check for JavaScript errors, promise errors, missing assets, network errors, rendering errors and warnings that point at bugs. Fix everything found.

### Visual polish

Smooth animations, fire particles, snow particles, wind effects, screen shake, impact effects, sound effects, music if it fits, line-clear animations, score animations, attractive menus, a start screen, a pause overlay, a game-over screen, instructions, hazard warnings, high-score presentation, a responsive HUD. The dragon and the yeti feel like characters invading the game, not icons sliding across it. The clears feel satisfying and cinematic. None of it makes the board hard to see.

### Sound

Sound is part of the polish: the dragon's warning and roar, the fire, the yeti's warning, the snowball and the wind, the three clear effects, and music if it fits. Sound never blocks play, plays only after the player has interacted with the page, and can be muted with one control.

<!-- This is the Tetris ask used to test Nerd Genie end to end, word for word.
     It is the same file as scripts/nightly/asks/05-tetris.md. The nightly runner
     and the show runs replace <WORK> with the folder the game goes in, such as
     /home/jared/Desktop, and the folder name below with the run's own. -->

# Tater Tots Tetris

Build a complete, polished, visually stunning, playable **Tetris-style web game** called:

# **Tater Tots Tetris**

Put the whole project in the folder <WORK>, which already exists; where this ask says the Desktop folder or the project folder, it means <WORK>, under the name:

`Tater Tots Tetrisv1`

Put **all source code, tests, assets, sound effects, and supporting files** inside that folder.

---

# Development Methodology — Test-Driven Development Is Mandatory

Use a strict **Test-Driven Development (TDD)** approach for the entire project.

Do **not** simply build the game first and add tests afterward.

For each major system or feature:

1. Define the expected behavior.
2. Write automated tests for that behavior.
3. Run the tests and confirm the new tests fail for the expected reason.
4. Implement the minimum necessary functionality to make those tests pass.
5. Run the tests again.
6. Fix all failures.
7. Refactor and improve the implementation without breaking the tests.
8. Re-run the entire test suite.
9. Only move on when the current feature and all previous features pass.

Repeat this cycle throughout development.

**At every major milestone, run the complete test suite and make sure 100% of tests pass before continuing.**

Never ignore, disable, skip, or delete a failing test merely to make the test suite green.

---

# Core Game Requirements

Build the game as a browser-based application using HTML, CSS, and JavaScript/TypeScript as appropriate.

The game must be:

* Modern
* Responsive
* Visually polished
* Smooth
* Fast
* Fun
* Easy to understand
* Professionally presented

Give the game distinctive **Tater Tots Tetris** branding while keeping the board and gameplay easy to read.

Implement normal Tetris gameplay including:

* Falling pieces
* Piece spawning
* Left/right movement
* Soft drop
* Hard drop
* Rotation
* Collision detection
* Piece locking
* Line clearing
* Scoring
* Levels
* Increasing speed/difficulty
* Game over
* Restart
* Pause
* Next-piece preview
* Keyboard controls
* Score display
* Level display
* Lines-cleared display
* High scores using local storage

Outside of the special hazards described below, the game should behave like a polished normal Tetris game.

---

# Special Hazard #1 — Dragon Attack

Approximately **1 out of every 10 pieces**, statistically, should have the possibility of triggering a **Dragon Attack**.

The occurrence must be randomized rather than happening exactly every tenth piece.

When a Dragon Attack triggers:

1. Give the player a short visual and audio warning that the dragon is coming.
2. A large animated dragon flies in from above the game area.
3. The player has a brief amount of time to hurry and position the current Tetris piece.
4. The dragon flies over the board and breathes fire onto the active piece.
5. The fire locks the piece's:

   * Horizontal position
   * Rotation
6. The player immediately loses control of the piece.
7. The piece then crashes straight downward.
8. It continues vertically until collision with the existing stack or bottom.
9. It locks normally.
10. Normal gameplay continues with the next piece.

The Dragon Attack should include polished visual effects such as:

* Dragon flight animation
* Fire breath
* Flames
* Sparks
* Embers
* Heat effects
* Screen shake
* Dragon roar
* Warning effects
* Impact animation

The player should immediately understand:

**THE DRAGON IS COMING — MOVE FAST BEFORE IT LOCKS YOUR PIECE.**

---

# Special Hazard #2 — Abominable Snowman / Yeti

Add a second randomized hazard involving an **Abominable Snowman / Yeti**.

The Yeti should occasionally attack from either the left or right side.

Use approximately **10% probability by default**, but make the probability configurable.

When a Yeti Attack begins:

1. Give the player a short warning.
2. Randomly choose whether the Yeti appears from the left or right side.
3. Animate the Yeti entering from that side.
4. Give the player a brief opportunity to reposition the active piece.
5. The Yeti throws a giant snowball.
6. The Yeti unleashes a powerful blast of polar wind across the board.

## Yeti From the Right

If the Yeti attacks from the:

`RIGHT`

The wind blows:

`RIGHT → LEFT`

The current piece is pushed **as far left as legally possible**.

## Yeti From the Left

If the Yeti attacks from the:

`LEFT`

The wind blows:

`LEFT → RIGHT`

The current piece is pushed **as far right as legally possible**.

After the wind hits:

1. Move the piece as far sideways as collision rules legally allow.
2. Lock its horizontal position.
3. Lock its rotation.
4. Disable player control for that piece.
5. Force the piece to fall vertically.
6. Continue falling until collision.
7. Lock it normally.
8. Continue normal gameplay.

The wind must respect the board and existing blocks.

Never:

* Push a piece outside the board.
* Push a piece through existing blocks.
* Place a piece inside an occupied location.

The Yeti attack should include polished effects such as:

* Animated Yeti
* Giant snowball
* Blowing snow
* Snow particles
* Frost
* Ice effects
* Wind streaks
* Screen shake
* Appropriate sound effects

The player should immediately understand:

**YETI ON LEFT = PIECE GETS BLOWN RIGHT**

**YETI ON RIGHT = PIECE GETS BLOWN LEFT**

---

# Line Clear Animation System

When a line is completed and cleared, the line should **not** just vanish instantly.

Instead, every cleared line must randomly use **one of three dramatic clear animations**.

When a line clears, randomly choose one of the following effects:

## 1. Freeze + Explode + Disintegrate

The completed line:

1. Instantly freezes with an icy/frosted effect.
2. Pauses very briefly for emphasis.
3. Explodes.
4. Breaks apart and disintegrates into particles/fragments.

Visual ideas:

* Frost spreading across the line
* Ice crystals
* Cold shimmer
* Sharp icy crack effect
* Shattering particles
* Debris/disintegration effect

## 2. Bomb Explosion

The completed line clears with a bomb-style explosion.

Visual ideas:

* Flash
* Explosion burst
* Shockwave
* Debris
* Smoke
* Screen shake
* Impact sound

## 3. Fire Burn

The completed line catches fire and burns away.

Visual ideas:

* Flames spreading across the line
* Burning glow
* Ember particles
* Smoke
* Charring effect
* Ash/disintegration
* Burning sound effect

## Rules for Line Clear Effects

* The effect should be selected randomly from those three line-clear styles.
* The randomness should feel varied and fun.
* The animation must remain clear and readable.
* The effect must not break gameplay timing.
* The board state must remain logically correct.
* Cleared lines must still resolve properly for:

  * Single-line clears
  * Multi-line clears
  * Back-to-back clears
  * Clears triggered after forced drops from Dragon or Yeti hazards

If multiple lines clear at once, the implementation may either:

* Apply the same chosen effect to all cleared lines in that clear event, or
* Randomize each cleared line independently

Either approach is acceptable as long as it looks polished and behaves consistently.

The line clear effects should feel satisfying, dramatic, and high quality, but they must **not** obscure gameplay so badly that the board becomes confusing.

---

# Hazard Architecture

Build the hazard system as proper game logic rather than fake animations layered on top of the game.

Use explicit game states similar to:

```text
NORMAL
DRAGON_WARNING
DRAGON_ATTACK
YETI_WARNING
YETI_ATTACK
FORCED_DROP
PIECE_LOCKED
LINE_CLEAR_ANIMATING
```

Only **one major hazard may affect a piece at a time**.

Never allow a Dragon Attack and Yeti Attack to affect the same piece simultaneously.

Hazard state must be associated with the correct active piece.

A delayed timer or animation must **never accidentally affect the next piece**.

If a player hard-drops a piece before a hazard reaches it, cleanly cancel or resolve that hazard without corrupting gameplay.

Centralize important configuration values such as:

```text
DRAGON_CHANCE
YETI_CHANCE
DRAGON_WARNING_TIME
YETI_WARNING_TIME
FORCED_DROP_SPEED
LINE_CLEAR_EFFECT_MODE
LINE_CLEAR_ANIMATION_DURATION
```

Make these easy to adjust while balancing the game.

---

# Required Automated Testing

Build a comprehensive automated test suite using the TDD process.

At minimum, tests must cover:

## Core Tetris

* Board initialization
* Piece generation
* Piece spawning
* Left movement
* Right movement
* Rotation
* Wall collision
* Block collision
* Bottom collision
* Soft drop
* Hard drop
* Piece locking
* Line detection
* Line clearing
* Multiple-line clearing
* Score calculation
* Level progression
* Increasing game speed
* Game-over detection
* Restart
* Pause
* Next-piece logic

## Dragon

* Dragon probability logic
* Dragon event triggering
* Dragon warning state
* Player controls during warning
* Dragon attack transition
* Rotation lock
* Horizontal movement lock
* Forced vertical drop
* Collision during forced drop
* Piece locking after forced drop
* Hazard cleanup
* Hazard cancellation if the piece has already locked

## Yeti

* Yeti probability logic
* Yeti event triggering
* Randomized attack direction
* Left-side Yeti pushes right
* Right-side Yeti pushes left
* Maximum legal horizontal displacement
* Collision detection during wind movement
* Rotation lock
* Horizontal movement lock
* Forced vertical drop
* Piece locking after forced drop
* Hazard cleanup

## Line Clear Effects

* Completed line detection
* Triggering of line clear animation state
* Random selection among the three line-clear effect types
* Freeze/disintegrate effect selection
* Bomb explosion effect selection
* Fire burn effect selection
* Logical line removal after each effect
* Correct collapse of rows after animation completes
* Multi-line clears still resolve correctly
* Scoring still resolves correctly after animation
* Line clear effects do not corrupt the board state
* Line clear effects do not interfere with next-piece spawning
* Line clear effects work correctly after Dragon forced drops
* Line clear effects work correctly after Yeti forced drops

## Hazard Safety

Test specifically that:

* Dragon and Yeti cannot trigger simultaneously.
* A hazard cannot affect the wrong piece.
* A hazard cannot survive into the next piece.
* Restart clears all hazard timers.
* Game over clears all hazard timers.
* Pausing does not corrupt hazard state.
* Repeated rapid inputs do not break forced movement.
* Timers cannot cause race conditions.
* Animation timing cannot modify game logic incorrectly.

---

# Deterministic Testing

Random gameplay systems must be testable.

Use either:

* An injectable random-number generator
* A seeded random-number generator
* Mockable randomness

Automated tests must **never rely on random luck**.

A Dragon probability test should not need to run repeatedly hoping that a dragon eventually appears.

A Yeti direction test should be able to deterministically force either direction.

A line-clear effect test should be able to deterministically force:

* Freeze/disintegrate
* Bomb explosion
* Fire burn

---

# Continuous Test Discipline

Throughout development:

**WRITE TEST → RUN TEST → IMPLEMENT → RUN TEST → FIX → REFACTOR → RUN ALL TESTS**

Do this repeatedly.

After implementing each significant feature:

```text
RUN THE FULL TEST SUITE
```

If any test fails:

```text
STOP
DEBUG THE FAILURE
FIX THE CODE
RUN THE TESTS AGAIN
```

Do not continue building new functionality while known automated tests are failing.

Before beginning final visual QA:

# EVERY AUTOMATED TEST MUST PASS.

---

# Browser Play Testing

Only after the automated test suite passes completely, launch the game in **Google Chrome**.

Control Chrome like a human user and actually play **Tater Tots Tetris**.

Do not merely verify that the page loads.

Play enough games to exercise the systems.

Test:

* Movement
* Rotation
* Soft drop
* Hard drop
* Line clearing
* Scoring
* Level progression
* Increasing speed
* Pause
* Restart
* Game over
* High scores
* Dragon attacks
* Yeti attacks
* Dragon warning timing
* Yeti warning timing
* Forced movement
* Forced drops
* Hazard collision behavior
* Rapid keyboard input
* Edge-of-board behavior
* Complex block stacks
* All three line-clear animation types
* Multi-line clears with animations
* Whether line-clear animations feel satisfying and readable

For development and QA, it is acceptable to create temporary developer controls that allow manually forcing:

```text
Trigger Dragon
Trigger Yeti From Left
Trigger Yeti From Right
Trigger Freeze/Disintegrate Line Clear
Trigger Bomb Line Clear
Trigger Fire Burn Line Clear
```

Use these controls to repeatedly test the special mechanics.

Remove or hide these developer controls in the finished user-facing game.

---

# Mandatory Visual QA

After functional browser testing, perform a dedicated **visual QA pass**.

Inspect the game as a human player would.

Test multiple browser sizes, including:

* Large desktop
* Standard laptop
* Narrow desktop window
* Tablet-like dimensions
* Mobile-like dimensions if the layout supports them

Look specifically for:

* Broken layouts
* Ugly spacing
* Alignment problems
* Bad typography
* Poor color contrast
* Board scaling issues
* Controls or HUD overlapping the board
* Clipped Dragon animations
* Clipped Yeti animations
* Fire effects covering important information
* Snow effects covering important information
* Broken line-clear animations
* Line clear effects that feel weak or unfinished
* Broken transitions
* Janky movement
* Low frame rate
* Incorrect responsive behavior
* Elements extending outside the viewport
* Inconsistent styling
* Pixelated or low-quality visuals
* Bad animation timing
* Pieces visually separating from their logical board position
* Screen shake that is too aggressive
* Unclear hazard warnings
* Any unfinished-looking element

Open the browser developer console and check for:

* JavaScript errors
* Promise errors
* Missing assets
* Network errors
* Rendering errors
* Warnings that indicate bugs

Fix everything discovered.

---

# Visual Polish

Make this feel like a real small indie game rather than a coding demo.

Use polish where appropriate:

* Smooth animations
* Fire particle effects
* Snow particle effects
* Wind effects
* Screen shake
* Impact effects
* Sound effects
* Music if appropriate
* Line-clear animations
* Score animations
* Attractive menus
* Start screen
* Pause overlay
* Game-over screen
* Instructions
* Hazard warnings
* High-score presentation
* Responsive HUD

The Dragon and Yeti should feel like **actual characters invading the game**, not simple icons sliding across the screen.

The line clears should feel satisfying and cinematic.

They should be dramatic, funny, readable, and exciting.

Do not allow visual effects to make the core Tetris board difficult to see.

---

# Final Regression Testing

After visual QA and browser testing, you will almost certainly have changed code.

Therefore:

# RUN THE ENTIRE AUTOMATED TEST SUITE AGAIN.

Every test must pass.

If even **one test fails**:

1. Investigate the failure.
2. Determine whether the bug came from the implementation or the QA changes.
3. Fix it correctly.
4. Re-run the complete test suite.
5. Continue until every test passes.

Then:

1. Re-launch the game in Chrome.
2. Perform another short gameplay test.
3. Trigger both Dragon and Yeti attacks.
4. Trigger all three line-clear animation types.
5. Verify no regression was introduced.
6. Check the console again.
7. Perform one final visual inspection.

Repeat this cycle as necessary:

```text
AUTOMATED TESTS
      ↓
BROWSER TESTING
      ↓
VISUAL QA
      ↓
FIX PROBLEMS
      ↓
AUTOMATED REGRESSION TESTS
      ↓
BROWSER RE-TEST
      ↓
FINAL QA
```

Do **not** consider the project finished until this loop produces a clean result.

---

# Final Acceptance Criteria

The project is complete only when:

* 100% of automated tests pass.
* There are no skipped tests hiding known failures.
* Core Tetris gameplay works correctly.
* Dragon attacks work correctly.
* Yeti attacks work correctly from both directions.
* Hazard probability logic works correctly.
* Forced drops work correctly.
* Collision detection works correctly.
* Line clearing works correctly.
* All three line-clear animations work correctly.
* Freeze/disintegrate clears work correctly.
* Bomb clears work correctly.
* Fire burn clears work correctly.
* Scoring works correctly.
* Restart completely resets the game.
* Game-over behavior works correctly.
* No stale hazard timers remain.
* The game has been manually played in Chrome.
* Dragon attacks have been manually tested.
* Both Yeti directions have been manually tested.
* All three line-clear animations have been manually tested.
* Multiple browser sizes have been visually inspected.
* There are no console errors.
* There are no obvious graphical bugs.
* Animations are smooth.
* Controls are responsive.
* The game looks professionally finished.
* Final regression tests pass after all visual QA fixes.

# Do not stop at the first working version.

Continue using the full:

**TDD → Implementation → Automated Testing → Browser Play Testing → Visual QA → Fixes → Regression Testing → Final Visual QA**

cycle until **Tater Tots Tetris** is stable, polished, visually impressive, fun to play, and every automated test passes.

You are authorized to autonomously:

* Create and edit files
* Use the project folder <WORK>
* Install required local development dependencies
* Execute terminal commands
* Run local servers
* Run automated tests
* Inspect test output
* Open Google Chrome
* Control the browser
* Play the game
* Inspect the browser console
* Perform visual QA
* Modify the implementation
* Re-run tests
* Continue iterating until every requirement and acceptance criterion has been satisfied

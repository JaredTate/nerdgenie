<!-- EX_PROMPT_2_TIC_TAC_TOE_VS_COMPUTER.md: the tic-tac-toe ask with a computer
     opponent, written as a work order, the shape PROMPT_TEMPLATE_GUIDE.md explains.
     EX_PROMPT_1_TIC_TAC_TOE.md is the same game for two players; the unbeatable
     computer, minimax proved by a test that plays every opening, is what makes
     this one several times harder for a small model.
     Difficulty 3 of 10. Expected: forty minutes to two hours, six tasks; the computer opponent is most of it, and the time depends on the GPU and the model. -->

# Tic Tac Toe

## Goal

Build a complete, visually stunning, browser-based tic-tac-toe game from scratch: two players at one keyboard or one player against an unbeatable computer, on a board that looks and feels like a small polished product, with smooth animations, a satisfying win, a scoreboard and a restart. The game logic and the computer opponent live in their own modules, separate from the rendering, and every rule of them is proved by an automated test before it is built. The person who plays it should think: that is the best-looking tic-tac-toe I have seen.

## Where

Create a new folder on the Desktop named exactly `Tic Tac Toe`, so the project lives at `~/Desktop/Tic Tac Toe`. Make it if it is not there. Put all source code, tests and assets inside it, and nothing anywhere else. Serve the game from that folder on port 8096.

## Done when

1. Every automated test passes and none is skipped. [tests pass: npm test]
2. The game loads in the browser and shows the board. [shows: "Tic Tac Toe" at http://127.0.0.1:8096]
3. In the Chrome window on the screen, each of these was done, seen and photographed: a two-player game won by X, a draw, a game against the computer that the computer did not lose, the win line and its celebration, the scoreboard counting, a restart, and a new match.
4. At 1440, 768 and 390 wide the board is square, centred and readable, nothing overlaps, and the console shows no errors.
5. The whole test suite is green after the last change made during play testing and visual QA.

## Rules

These are the choices already made, so the work never has to make them.

- Tests first: write the test, watch it fail, write the code, watch it pass; the whole suite green before a task ends.
- Any framework or none. Plain HTML, CSS and JavaScript are enough here. Whatever you choose, `npm test` runs the whole suite and `npm start` serves the game on port 8096.
- The Chrome window on the screen, driven with the browser tools, is how the game is opened, played, photographed and checked. That window is how you see your work.
- Keep the game logic in a pure module the renderer only reads: the board, whose turn, a move, the winner and the winning line, a draw, and the computer's move by minimax, so every rule is tested without a screen.
- Keep the look in one theme file: colours, the mark shapes, the animation timings, so restyling is one edit.
- Expose the state on the page as `window.game` (board, turn, winner, scores, mode), so the browser tool can ask the page instead of reading pixels.
- When a test will not pass, write the failure and its cause into the record and take another route. The job is finished when every done line is met.

## Tasks

1. Scaffold: `package.json`, a test runner, `index.html`, one smoke test, `npm test` and `npm start` working on 8096. Done when the smoke test passes and the page is served. (Details: Engineering)
2. The game logic: an empty board, turns, a legal move, an illegal move refused, every winning line, a draw, restart, and the scoreboard across games. Done when the logic tests pass. (Details: The game, Tests required)
3. The computer opponent: minimax over the board so the computer never loses, playing X or O, with a first move that is not always the same corner. Done when a test plays every opening against it and it never loses. (Details: The computer, Tests required)
4. The board on screen: a square, centred board that fits any window, marks drawn with a smooth animation, hover on empty cells, the turn shown, a mode switch between two players and the computer, wired to the logic. Done when a game can be played with the mouse in Chrome and the console shows no errors. (Details: The look)
5. The win, the scoreboard and the restart: the winning line drawn across the three cells, a celebration that does not hide the board, the draw shown, the scores counting X, O and draws, restart and a new match. Done when done line 3 is met. (Details: The look)
6. Visual QA and the final regression: photograph the board at the three sizes and check each against done line 4, once; fix only what a done line names; then run the whole suite once more and finish. Done when done lines 4 and 5 are met. (Details: The look, Browser play testing)

## Details

### The game

A three-by-three board. X moves first. A move on a taken cell is refused. Three in a row, column or diagonal wins and the winning cells are known. A full board with no winner is a draw. A restart clears the board and keeps the scores; a new match clears the scores too. Two modes: two players at one keyboard taking turns, or one player against the computer, and the player may choose X or O.

### The computer

An unbeatable opponent by minimax, with a little variety: when several moves are equally good it picks one at random from an injectable random source, so a test can force its choice and a person does not see the same game every time.

### The look

Visually stunning for what it is: a deep, modern palette from the theme file, marks that draw themselves in with a short smooth animation, a gentle hover on empty cells, the winning line drawn across the board with a burst of light or particles that clears in a second, a scoreboard and the turn indicator that are readable at a glance, a mode switch, restart and new match buttons that look like part of the design, and everything centred and square at 1440, 768 and 390 wide. No sound is needed; a soft click on a move is a bonus if it is easy.

### Engineering

The logic and the computer in pure modules the renderer reads; one theme file; tests run after every change and a failing test fixed before anything else is built.

### Tests required

Empty board; turn order; a legal move; an illegal move refused; each of the eight winning lines with its cells; a draw; restart keeps the scores; new match clears them; the computer never loses from any opening, as X and as O; the computer's forced choice among equal moves; the mode switch; the scoreboard.

### Browser play testing

After the suite passes, open the game in the Chrome window and go through this list once, photographing each item as done line 3 asks:

- a two-player game to a win, with the win line and its celebration
- a game to a draw
- a game against the computer that the computer did not lose
- the mode switch, the scoreboard counting, a restart, a new match
- the board at 1440, 768 and 390 wide: square, centred, readable, nothing overlapping
- the console, with no errors

A fix is made only for something a done line names; then run the whole suite once more and finish.

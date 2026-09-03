# wordcount, a tool you can write yourself

`wordcount` counts the words in a piece of text. It exists to show the whole of the user-tool protocol in one short file you can read in a minute, and `docs/EXTENDING.md` section 2 is the guide it belongs to.

**Install it.** Copy the `wordcount` file into `~/.coeus/tools/`, make it executable with `chmod +x`, and restart the agent. Nothing in the repository changes: the registry reads that folder at startup and adds whatever it finds to the tools every model sees.

**How it works.** Run with `--describe` it prints a JSON tool specification: a name, a description under the forty-word cap, the input fields, and the permission class `R`, which means the tool only reads. Run with no arguments it reads one JSON object of the model's arguments from standard input and prints the result as plain text. That is the entire protocol; a tool in any language that does those two things works.

**Try it by hand.**

    ./wordcount --describe
    echo '{"text":"one two three"}' | ./wordcount

The second line prints `3`.

**What the harness adds.** A user tool goes through the same permission function as a built-in one, its result is capped and marked as data rather than instructions, and a tool whose description will not parse or runs over the cap is skipped at startup with one logged line naming the file and the problem, so one bad script never stops the agent from starting.

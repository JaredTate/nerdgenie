# The model's behaviour on 6 September 2026, measured

Three reports written on the afternoon of 6 September 2026 while the fresh
Tetris run went on, by three agents working from the event logs, the
repository's own research documents, and the web.

- `behaviour.md` and `tables.md`: 32 tasks, 2,173 rounds and 685 minutes of
  model time across the three logs of the day (runs five and six and the
  live run), with a ranked list of what wasted rounds, tokens and time.
  `analyse.py` produced them from copies of the logs; `rounds.tsv` was too
  large to keep.
- `ideas-from-the-documents.md`: every idea in LLM_RESEARCH.md,
  STATE_RESEARCH.md, the plan and the doom-loop and metrics studies, with its
  status, and eight candidates that were never built.
- `ideas-from-the-web.md`: forty-five sources from 2025 and 2026 on context
  engineering, small models in agent loops, long-horizon memory, llama.cpp
  specifics and token accounting, and eight candidates with their evidence.

The five ideas put to the owner from these are in `../../ideas/2026-09-06-five-ideas.md`.

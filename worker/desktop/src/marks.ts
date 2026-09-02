// The numbered controls the model points at. The idea of numbering the
// actionable controls on a picture and letting the model act by number is
// Hermes' set-of-marks capture, at ~/Code/hermes-agent/tools/computer_use/backend.py;
// the numbers, roles, and labels here are written fresh for Coeus.

/** One numbered control on the granted application's window. */
export interface Mark {
  /** The number drawn on the picture, handed out fresh on every screenshot. */
  number: number
  /** What kind of control it is, such as "button" or "text box". */
  role: string
  /** The label on it, trimmed to one short line. */
  name: string
}

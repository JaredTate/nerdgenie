// Everything the worker refuses to do more of. Rule seven in
// docs/WORK_PLAN.md Part 1 is that every loop has a limit, every wait a
// timeout, and every buffer a cap, and this is where the desktop worker's are.

/** The most characters one `type` call may carry. */
export const maximumTypedCharacters = 10_000

/** The most base64 characters a window's picture may come to, which is 4 MB. */
export const maximumPictureLength = 4 * 1024 * 1024

/** The most windows one screenshot names, because each title is a line of the model's context. */
export const maximumWindowsListed = 50

/** The longest an application name may be before it is refused. */
export const maximumApplicationNameLength = 200

/** The most readings one settle wait will take, however short the waits are. */
export const maximumSettleReadings = 60

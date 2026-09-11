/**
 * Evaluates code in the current environment. This function matches centralized
 * evaluation as implemented in TypeBox 1.x.
 */
export function Evaluate(...args) {
    return new globalThis.Function(...args);
}

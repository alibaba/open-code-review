/** The base Error type thrown for all TypeBox exceptions  */
export class TypeBoxError extends Error {
    constructor(message) {
        super(message);
    }
}

// ------------------------------------------------------------------
// JsonTypeBuilder
// ------------------------------------------------------------------
export { JsonTypeBuilder } from './json.mjs';
// ------------------------------------------------------------------
// JavaScriptTypeBuilder
// ------------------------------------------------------------------
import * as TypeBuilder from './type.mjs';
import { JavaScriptTypeBuilder } from './javascript.mjs';
/** JavaScript Type Builder with Static Resolution for TypeScript */
const Type = TypeBuilder;
export { JavaScriptTypeBuilder };
export { Type };

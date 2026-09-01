import { TypeSystemPolicy } from '../../system/policy.mjs';
import { Immutable } from './immutable.mjs';
import { Clone } from '../clone/value.mjs';
/** Creates TypeBox schematics using the configured InstanceMode */
export function CreateType(schema, options) {
    const result = options !== undefined ? { ...options, ...schema } : schema;
    switch (TypeSystemPolicy.InstanceMode) {
        case 'freeze':
            return Immutable(result);
        case 'clone':
            return Clone(result);
        default:
            return result;
    }
}

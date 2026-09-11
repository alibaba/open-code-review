import { extend } from '../utils';
import logger from '../logger';

const loggedProperties = Object.create(null);

export function createProtoAccessControl(runtimeOptions) {
  // Create an object with "null"-prototype to avoid truthy results on
  // prototype properties.
  const propertyWhiteList = Object.create(null);
  // eslint-disable-next-line no-proto
  propertyWhiteList['__proto__'] = false;
  extend(propertyWhiteList, runtimeOptions.allowedProtoProperties);

  const methodWhiteList = Object.create(null);
  methodWhiteList['constructor'] = false;
  methodWhiteList['__defineGetter__'] = false;
  methodWhiteList['__defineSetter__'] = false;
  methodWhiteList['__lookupGetter__'] = false;
  methodWhiteList['__lookupSetter__'] = false;
  extend(methodWhiteList, runtimeOptions.allowedProtoMethods);

  return {
    properties: {
      whitelist: propertyWhiteList,
      defaultValue: runtimeOptions.allowProtoPropertiesByDefault
    },
    methods: {
      whitelist: methodWhiteList,
      defaultValue: runtimeOptions.allowProtoMethodsByDefault
    }
  };
}

export function resultIsAllowed(result, protoAccessControl, propertyName) {
  if (typeof result === 'function') {
    return checkWhiteList(protoAccessControl.methods, propertyName);
  } else {
    return checkWhiteList(protoAccessControl.properties, propertyName);
  }
}

function checkWhiteList(protoAccessControlForType, propertyName) {
  if (protoAccessControlForType.whitelist[propertyName] !== undefined) {
    return protoAccessControlForType.whitelist[propertyName] === true;
  }
  if (protoAccessControlForType.defaultValue !== undefined) {
    return protoAccessControlForType.defaultValue;
  }
  logUnexpecedPropertyAccessOnce(propertyName);
  return false;
}

function logUnexpecedPropertyAccessOnce(propertyName) {
  if (loggedProperties[propertyName] !== true) {
    loggedProperties[propertyName] = true;
    logger.log(
      'error',
      `Handlebars: Access has been denied to resolve the property "${propertyName}" because it is not an "own property" of its parent.\n` +
        `You can add a runtime option to disable the check or this warning:\n` +
        `See https://handlebarsjs.com/api-reference/runtime-options.html#options-to-control-prototype-access for details`
    );
  }
}

export function resetLoggedProperties() {
  Object.keys(loggedProperties).forEach(propertyName => {
    delete loggedProperties[propertyName];
  });
}

import parser from './parser';
import WhitespaceControl from './whitespace-control';
import * as Helpers from './helpers';
import Exception from '../exception';
import { extend } from '../utils';

export { parser };

let yy = {};
extend(yy, Helpers);

export function parseWithoutProcessing(input, options) {
  // Just return if an already-compiled AST was passed in.
  if (input.type === 'Program') {
    // When a pre-parsed AST is passed in, validate all node values to prevent
    // code injection via type-confused literals.
    validateInputAst(input);
    return input;
  }

  parser.yy = yy;

  // Altering the shared object here, but this is ok as parser is a sync operation
  yy.locInfo = function(locInfo) {
    return new yy.SourceLocation(options && options.srcName, locInfo);
  };

  let ast = parser.parse(input);

  return ast;
}

export function parse(input, options) {
  let ast = parseWithoutProcessing(input, options);
  let strip = new WhitespaceControl(options);

  return strip.accept(ast);
}

function validateInputAst(ast) {
  validateAstNode(ast);
}

function validateAstNode(node) {
  if (node == null) {
    return;
  }

  if (Array.isArray(node)) {
    node.forEach(validateAstNode);
    return;
  }

  if (typeof node !== 'object') {
    return;
  }

  if (node.type === 'PathExpression') {
    if (!isValidDepth(node.depth)) {
      throw new Exception(
        'Invalid AST: PathExpression.depth must be an integer'
      );
    }
    if (!Array.isArray(node.parts)) {
      throw new Exception('Invalid AST: PathExpression.parts must be an array');
    }
    for (let i = 0; i < node.parts.length; i++) {
      if (typeof node.parts[i] !== 'string') {
        throw new Exception(
          'Invalid AST: PathExpression.parts must only contain strings'
        );
      }
    }
  } else if (node.type === 'NumberLiteral') {
    if (typeof node.value !== 'number' || !isFinite(node.value)) {
      throw new Exception('Invalid AST: NumberLiteral.value must be a number');
    }
  } else if (node.type === 'BooleanLiteral') {
    if (typeof node.value !== 'boolean') {
      throw new Exception(
        'Invalid AST: BooleanLiteral.value must be a boolean'
      );
    }
  }

  Object.keys(node).forEach(propertyName => {
    if (propertyName === 'loc') {
      return;
    }
    validateAstNode(node[propertyName]);
  });
}

function isValidDepth(depth) {
  return (
    typeof depth === 'number' &&
    isFinite(depth) &&
    Math.floor(depth) === depth &&
    depth >= 0
  );
}

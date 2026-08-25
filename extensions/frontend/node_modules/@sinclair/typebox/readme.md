<div align='center'>

<h1>Sinclair TypeBox</h1>

<p>Json Schema Type Builder with Static Type Resolution for TypeScript</p>

<img src="https://raw.githubusercontent.com/sinclairzx81/sinclair-typebox/refs/heads/main/typebox.png" />

<br />
<br />

[![npm version](https://badge.fury.io/js/%40sinclair%2Ftypebox.svg)](https://badge.fury.io/js/%40sinclair%2Ftypebox)
[![Downloads](https://img.shields.io/npm/dm/%40sinclair%2Ftypebox.svg)](https://www.npmjs.com/package/%40sinclair%2Ftypebox)
[![Build](https://github.com/sinclairzx81/typebox/actions/workflows/build.yml/badge.svg)](https://github.com/sinclairzx81/typebox/actions/workflows/build.yml)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

</div>


<a name="Install"></a>

## Install

For the latest version use [TypeBox 1.x](https://github.com/sinclairzx81/typebox) 

```bash
$ npm install @sinclair/typebox                     # TypeBox 0.x - Long Term Support

$ npm install typebox                               # TypeBox 1.x - Latest
```

## Example

```typescript
import { Type, type Static } from '@sinclair/typebox'

const T = Type.Object({                              // const T = {
  x: Type.Number(),                                  //   type: 'object',
  y: Type.Number(),                                  //   required: ['x', 'y', 'z'],
  z: Type.Number()                                   //   properties: {
})                                                   //     x: { type: 'number' },
                                                     //     y: { type: 'number' },
                                                     //     z: { type: 'number' }
                                                     //   }
                                                     // }

type T = Static<typeof T>                            // type T = {
                                                     //   x: number,
                                                     //   y: number,
                                                     //   z: number
                                                     // }
```

<a name="Overview"></a>

## Overview

TypeBox is a runtime type builder that creates in-memory Json Schema objects that infer as TypeScript types. The schematics produced by this library are designed to match the static type checking rules of the TypeScript compiler. TypeBox offers a unified type that can be statically checked by TypeScript and runtime asserted using standard Json Schema validation.

This library is designed to allow Json Schema to compose similar to how types compose within TypeScript's type system. It can be used as a simple tool to build up complex schematics or integrated into REST and RPC services to help validate data received over the wire.

License MIT

## Contents
- [Install](#install)
- [Overview](#overview)
- [Usage](#usage)
- [Types](#types)
  - [Json](#types-json)
  - [JavaScript](#types-javascript)
  - [Options](#types-options)
  - [Properties](#types-properties)
  - [Generics](#types-generics)
  - [Recursive](#types-recursive)
  - [Modules](#types-modules)
  - [Template Literal](#types-template-literal)
  - [Indexed](#types-indexed)
  - [Mapped](#types-mapped)
  - [Conditional](#types-conditional)
  - [Transform](#types-transform)
  - [Guard](#types-guard)
  - [Unsafe](#types-unsafe)
- [Values](#values)
  - [Assert](#values-assert)
  - [Create](#values-create)
  - [Clone](#values-clone)
  - [Check](#values-check)
  - [Convert](#values-convert)
  - [Default](#values-default)
  - [Clean](#values-clean)
  - [Cast](#values-cast)
  - [Decode](#values-decode)
  - [Encode](#values-decode)
  - [Parse](#values-parse)
  - [Equal](#values-equal)
  - [Hash](#values-hash)
  - [Diff](#values-diff)
  - [Patch](#values-patch)
  - [Errors](#values-errors)
  - [Mutate](#values-mutate)
  - [Pointer](#values-pointer)
- [Syntax](#syntax)
  - [Create](#syntax-create)
  - [Parameters](#syntax-parameters)
  - [Generics](#syntax-generics)
  - [Options](#syntax-options)
  - [NoInfer](#syntax-no-infer)
- [TypeRegistry](#typeregistry)
  - [Type](#typeregistry-type)
  - [Format](#typeregistry-format)
- [TypeCheck](#typecheck)
  - [Ajv](#typecheck-ajv)
  - [TypeCompiler](#typecheck-typecompiler)
- [TypeMap](#typemap)
  - [Usage](#typemap-usage)
- [TypeSystem](#typesystem)
  - [Policies](#typesystem-policies)
- [Error Function](#error-function)
- [Workbench](#workbench)
- [Codegen](#codegen)
- [Ecosystem](#ecosystem)
- [Benchmark](#benchmark)
  - [Compile](#benchmark-compile)
  - [Validate](#benchmark-validate)
  - [Compression](#benchmark-compression)
- [Contribute](#contribute)

<a name="usage"></a>

## Usage

The following shows general usage.

```typescript
import { Type, type Static } from '@sinclair/typebox'

//--------------------------------------------------------------------------------------------
//
// Let's say you have the following type ...
//
//--------------------------------------------------------------------------------------------

type T = {
  id: string,
  name: string,
  timestamp: number
}

//--------------------------------------------------------------------------------------------
//
// ... you can express this type in the following way.
//
//--------------------------------------------------------------------------------------------

const T = Type.Object({                              // const T = {
  id: Type.String(),                                 //   type: 'object',
  name: Type.String(),                               //   properties: {
  timestamp: Type.Integer()                          //     id: {
})                                                   //       type: 'string'
                                                     //     },
                                                     //     name: {
                                                     //       type: 'string'
                                                     //     },
                                                     //     timestamp: {
                                                     //       type: 'integer'
                                                     //     }
                                                     //   },
                                                     //   required: [
                                                     //     'id',
                                                     //     'name',
                                                     //     'timestamp'
                                                     //   ]
                                                     // }

//--------------------------------------------------------------------------------------------
//
// ... then infer back to the original static type this way.
//
//--------------------------------------------------------------------------------------------

type T = Static<typeof T>                            // type T = {
                                                     //   id: string,
                                                     //   name: string,
                                                     //   timestamp: number
                                                     // }

//--------------------------------------------------------------------------------------------
//
// ... or use the type to parse JavaScript values.
//
//--------------------------------------------------------------------------------------------

import { Value } from '@sinclair/typebox/value'

const R = Value.Parse(T, value)                      // const R: {
                                                     //   id: string,
                                                     //   name: string,
                                                     //   timestamp: number
                                                     // }
```

<a name='types'></a>

## Types

TypeBox types are Json Schema fragments that compose into more complex types. Each fragment is structured such that any Json Schema compliant validator can runtime assert a value the same way TypeScript will statically assert a type. TypeBox offers a set of Json Types which are used to create Json Schema compliant schematics as well as a JavaScript type set used to create schematics for constructs native to JavaScript.

<a name='types-json'></a>

### Json Types

The following table lists the supported Json types. These types are fully compatible with the Json Schema Draft 7 specification.

```typescript
┌────────────────────────────────┬─────────────────────────────┬────────────────────────────────┐
│ TypeBox                        │ TypeScript                  │ Json Schema                    │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Any()           │ type T = any                │ const T = { }                  │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Unknown()       │ type T = unknown            │ const T = { }                  │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.String()        │ type T = string             │ const T = {                    │
│                                │                             │   type: 'string'               │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Number()        │ type T = number             │ const T = {                    │
│                                │                             │   type: 'number'               │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Integer()       │ type T = number             │ const T = {                    │
│                                │                             │   type: 'integer'              │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Boolean()       │ type T = boolean            │ const T = {                    │
│                                │                             │   type: 'boolean'              │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Null()          │ type T = null               │ const T = {                    │
│                                │                             │   type: 'null'                 │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Literal(42)     │ type T = 42                 │ const T = {                    │
│                                │                             │   const: 42,                   │
│                                │                             │   type: 'number'               │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Array(          │ type T = number[]           │ const T = {                    │
│   Type.Number()                │                             │   type: 'array',               │
│ )                              │                             │   items: {                     │
│                                │                             │     type: 'number'             │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Object({        │ type T = {                  │ const T = {                    │
│   x: Type.Number(),            │   x: number,                │   type: 'object',              │
│   y: Type.Number()             │   y: number                 │   required: ['x', 'y'],        │
│ })                             │ }                           │   properties: {                │
│                                │                             │     x: {                       │
│                                │                             │       type: 'number'           │
│                                │                             │     },                         │
│                                │                             │     y: {                       │
│                                │                             │       type: 'number'           │
│                                │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Tuple([         │ type T = [number, number]   │ const T = {                    │
│   Type.Number(),               │                             │   type: 'array',               │
│   Type.Number()                │                             │   items: [{                    │
│ ])                             │                             │     type: 'number'             │
│                                │                             │   }, {                         │
│                                │                             │     type: 'number'             │
│                                │                             │   }],                          │
│                                │                             │   additionalItems: false,      │
│                                │                             │   minItems: 2,                 │
│                                │                             │   maxItems: 2                  │
│                                │                             │ }                              │
│                                │                             │                                │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ enum Foo {                     │ enum Foo {                  │ const T = {                    │
│   A,                           │   A,                        │   anyOf: [{                    │
│   B                            │   B                         │     type: 'number',            │
│ }                              │ }                           │     const: 0                   │
│                                │                             │   }, {                         │
│ const T = Type.Enum(Foo)       │ type T = Foo                │     type: 'number',            │
│                                │                             │     const: 1                   │
│                                │                             │   }]                           │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Const({         │ type T = {                  │ const T = {                    │
│   x: 1,                        │   readonly x: 1,            │   type: 'object',              │
│   y: 2,                        │   readonly y: 2             │   required: ['x', 'y'],        │
│ } as const)                    │ }                           │   properties: {                │
│                                │                             │     x: {                       │
│                                │                             │       type: 'number',          │
│                                │                             │       const: 1                 │
│                                │                             │     },                         │
│                                │                             │     y: {                       │
│                                │                             │       type: 'number',          │
│                                │                             │       const: 2                 │
│                                │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.KeyOf(          │ type T = keyof {            │ const T = {                    │
│   Type.Object({                │   x: number,                │   anyOf: [{                    │
│     x: Type.Number(),          │   y: number                 │     type: 'string',            │
│     y: Type.Number()           │ }                           │     const: 'x'                 │
│   })                           │                             │   }, {                         │
│ )                              │                             │     type: 'string',            │
│                                │                             │     const: 'y'                 │
│                                │                             │   }]                           │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Union([         │ type T = string | number    │ const T = {                    │
│   Type.String(),               │                             │   anyOf: [{                    │
│   Type.Number()                │                             │     type: 'string'             │
│ ])                             │                             │   }, {                         │
│                                │                             │     type: 'number'             │
│                                │                             │   }]                           │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Intersect([     │ type T = {                  │ const T = {                    │
│   Type.Object({                │   x: number                 │   allOf: [{                    │
│     x: Type.Number()           │ } & {                       │     type: 'object',            │
│   }),                          │   y: number                 │     required: ['x'],           │
│   Type.Object({                │ }                           │     properties: {              │
│     y: Type.Number()           │                             │       x: {                     │
│   })                           │                             │         type: 'number'         │
│ ])                             │                             │       }                        │
│                                │                             │     }                          │
│                                │                             │   }, {                         │
│                                │                             │     type: 'object',            |
│                                │                             │     required: ['y'],           │
│                                │                             │     properties: {              │
│                                │                             │       y: {                     │
│                                │                             │         type: 'number'         │
│                                │                             │       }                        │
│                                │                             │     }                          │
│                                │                             │   }]                           │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Composite([     │ type T = {                  │ const T = {                    │
│   Type.Object({                │   x: number,                │   type: 'object',              │
│     x: Type.Number()           │   y: number                 │   required: ['x', 'y'],        │
│   }),                          │ }                           │   properties: {                │
│   Type.Object({                │                             │     x: {                       │
│     y: Type.Number()           │                             │       type: 'number'           │
│   })                           │                             │     },                         │
│ ])                             │                             │     y: {                       │
│                                │                             │       type: 'number'           │
│                                │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Never()         │ type T = never              │ const T = {                    │
│                                │                             │   not: {}                      │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Not(            | type T = unknown            │ const T = {                    │
│   Type.String()                │                             │   not: {                       │
│ )                              │                             │     type: 'string'             │
│                                │                             │   }                            │
│                                │                             │ }                              │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Extends(        │ type T =                    │ const T = {                    │
│   Type.String(),               │  string extends number      │   const: false,                │
│   Type.Number(),               │    ? true                   │   type: 'boolean'              │
│   Type.Literal(true),          │    : false                  │ }                              │
│   Type.Literal(false)          │                             │                                │
│ )                              │                             │                                │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Extract(        │ type T = Extract<           │ const T = {                    │
│   Type.Union([                 │   string | number,          │   type: 'string'               │
│     Type.String(),             │   string                    │ }                              │
│     Type.Number(),             │ >                           │                                │
│   ]),                          │                             │                                │
│   Type.String()                │                             │                                │
│ )                              │                             │                                │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Exclude(        │ type T = Exclude<           │ const T = {                    │
│   Type.Union([                 │   string | number,          │   type: 'number'               │
│     Type.String(),             │   string                    │ }                              │
│     Type.Number(),             │ >                           │                                │
│   ]),                          │                             │                                │
│   Type.String()                │                             │                                │
│ )                              │                             │                                │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Mapped(         │ type T = {                  │ const T = {                    │
│   Type.Union([                 │   [_ in 'x' | 'y'] : number │   type: 'object',              │
│     Type.Literal('x'),         │ }                           │   required: ['x', 'y'],        │
│     Type.Literal('y')          │                             │   properties: {                │
│   ]),                          │                             │     x: {                       │
│   () => Type.Number()          │                             │       type: 'number'           │
│ )                              │                             │     },                         │
│                                │                             │     y: {                       │
│                                │                             │       type: 'number'           │
│                                │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const U = Type.Union([         │ type U = 'open' | 'close'   │ const T = {                    │
│   Type.Literal('open'),        │                             │   type: 'string',              │
│   Type.Literal('close')        │ type T = `on${U}`           │   pattern: '^on(open|close)$'  │
│ ])                             │                             │ }                              │
│                                │                             │                                │
│ const T = Type                 │                             │                                │
│   .TemplateLiteral([           │                             │                                │
│      Type.Literal('on'),       │                             │                                │
│      U                         │                             │                                │
│   ])                           │                             │                                │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Record(         │ type T = Record<            │ const T = {                    │
│   Type.String(),               │   string,                   │   type: 'object',              │
│   Type.Number()                │   number                    │   patternProperties: {         │
│ )                              │ >                           │     '^.*$': {                  │
│                                │                             │       type: 'number'           │
│                                │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Partial(        │ type T = Partial<{          │ const T = {                    │
│   Type.Object({                │   x: number,                │   type: 'object',              │
│     x: Type.Number(),          │   y: number                 │   properties: {                │
│     y: Type.Number()           | }>                          │     x: {                       │
│   })                           │                             │       type: 'number'           │
│ )                              │                             │     },                         │
│                                │                             │     y: {                       │
│                                │                             │       type: 'number'           │
│                                │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Required(       │ type T = Required<{         │ const T = {                    │
│   Type.Object({                │   x?: number,               │   type: 'object',              │
│     x: Type.Optional(          │   y?: number                │   required: ['x', 'y'],        │
│       Type.Number()            | }>                          │   properties: {                │
│     ),                         │                             │     x: {                       │
│     y: Type.Optional(          │                             │       type: 'number'           │
│       Type.Number()            │                             │     },                         │
│     )                          │                             │     y: {                       │
│   })                           │                             │       type: 'number'           │
│ )                              │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Pick(           │ type T = Pick<{             │ const T = {                    │
│   Type.Object({                │   x: number,                │   type: 'object',              │
│     x: Type.Number(),          │   y: number                 │   required: ['x'],             │
│     y: Type.Number()           │ }, 'x'>                     │   properties: {                │
│   }), ['x']                    |                             │     x: {                       │
│ )                              │                             │       type: 'number'           │
│                                │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Omit(           │ type T = Omit<{             │ const T = {                    │
│   Type.Object({                │   x: number,                │   type: 'object',              │
│     x: Type.Number(),          │   y: number                 │   required: ['y'],             │
│     y: Type.Number()           │ }, 'x'>                     │   properties: {                │
│   }), ['x']                    |                             │     y: {                       │
│ )                              │                             │       type: 'number'           │
│                                │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Index(          │ type T = {                  │ const T = {                    │
│   Type.Object({                │   x: number,                │   type: 'number'               │
│     x: Type.Number(),          │   y: string                 │ }                              │
│     y: Type.String()           │ }['x']                      │                                │
│   }), ['x']                    │                             │                                │
│ )                              │                             │                                │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const A = Type.Tuple([         │ type A = [0, 1]             │ const T = {                    │
│   Type.Literal(0),             │ type B = [2, 3]             │   type: 'array',               │
│   Type.Literal(1)              │ type T = [                  │   items: [                     │
│ ])                             │   ...A,                     │     { const: 0 },              │
│ const B = Type.Tuple([         │   ...B                      │     { const: 1 },              │
|   Type.Literal(2),             │ ]                           │     { const: 2 },              │
|   Type.Literal(3)              │                             │     { const: 3 }               │
│ ])                             │                             │   ],                           │
│ const T = Type.Tuple([         │                             │   additionalItems: false,      │
|   ...Type.Rest(A),             │                             │   minItems: 4,                 │
|   ...Type.Rest(B)              │                             │   maxItems: 4                  │
│ ])                             │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Uncapitalize(   │ type T = Uncapitalize<      │ const T = {                    │
│   Type.Literal('Hello')        │   'Hello'                   │   type: 'string',              │
│ )                              │ >                           │   const: 'hello'               │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Capitalize(     │ type T = Capitalize<        │ const T = {                    │
│   Type.Literal('hello')        │   'hello'                   │   type: 'string',              │
│ )                              │ >                           │   const: 'Hello'               │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Uppercase(      │ type T = Uppercase<         │ const T = {                    │
│   Type.Literal('hello')        │   'hello'                   │   type: 'string',              │
│ )                              │ >                           │   const: 'HELLO'               │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Lowercase(      │ type T = Lowercase<         │ const T = {                    │
│   Type.Literal('HELLO')        │   'HELLO'                   │   type: 'string',              │
│ )                              │ >                           │   const: 'hello'               │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const R = Type.Ref('T')        │ type R = unknown            │ const R = { $ref: 'T' }        │
│                                │                             │                                │
└────────────────────────────────┴─────────────────────────────┴────────────────────────────────┘
```

<a name='types-javascript'></a>

### JavaScript Types

TypeBox provides an extended type set that can be used to create schematics for common JavaScript constructs. These types can not be used with any standard Json Schema validator; but can be used to frame schematics for interfaces that may receive Json validated data. JavaScript types are prefixed with the `[JavaScript]` JSDoc comment for convenience. The following table lists the supported types.

```typescript
┌────────────────────────────────┬─────────────────────────────┬────────────────────────────────┐
│ TypeBox                        │ TypeScript                  │ Extended Schema                │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Constructor([   │ type T = new (              │ const T = {                    │
│   Type.String(),               │  arg0: string,              │   type: 'Constructor',         │
│   Type.Number()                │  arg0: number               │   parameters: [{               │
│ ], Type.Boolean())             │ ) => boolean                │     type: 'string'             │
│                                │                             │   }, {                         │
│                                │                             │     type: 'number'             │
│                                │                             │   }],                          │
│                                │                             │   returns: {                   │
│                                │                             │     type: 'boolean'            │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Function([      │ type T = (                  │ const T = {                    │
|   Type.String(),               │  arg0: string,              │   type: 'Function',            │
│   Type.Number()                │  arg1: number               │   parameters: [{               │
│ ], Type.Boolean())             │ ) => boolean                │     type: 'string'             │
│                                │                             │   }, {                         │
│                                │                             │     type: 'number'             │
│                                │                             │   }],                          │
│                                │                             │   returns: {                   │
│                                │                             │     type: 'boolean'            │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Promise(        │ type T = Promise<string>    │ const T = {                    │
│   Type.String()                │                             │   type: 'Promise',             │
│ )                              │                             │   item: {                      │
│                                │                             │     type: 'string'             │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T =                      │ type T =                    │ const T = {                    │
│   Type.AsyncIterator(          │   AsyncIterableIterator<    │   type: 'AsyncIterator',       │
│     Type.String()              │    string                   │   items: {                     │
│   )                            │   >                         │     type: 'string'             │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Iterator(       │ type T =                    │ const T = {                    │
│   Type.String()                │   IterableIterator<string>  │   type: 'Iterator',            │
│ )                              │                             │   items: {                     │
│                                │                             │     type: 'string'             │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.RegExp(/abc/i)  │ type T = string             │ const T = {                    │
│                                │                             │   type: 'RegExp'               │
│                                │                             │   source: 'abc'                │
│                                │                             │   flags: 'i'                   │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Uint8Array()    │ type T = Uint8Array         │ const T = {                    │
│                                │                             │   type: 'Uint8Array'           │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Date()          │ type T = Date               │ const T = {                    │
│                                │                             │   type: 'Date'                 │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Undefined()     │ type T = undefined          │ const T = {                    │
│                                │                             │   type: 'undefined'            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Symbol()        │ type T = symbol             │ const T = {                    │
│                                │                             │   type: 'symbol'               │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.BigInt()        │ type T = bigint             │ const T = {                    │
│                                │                             │   type: 'bigint'               │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Void()          │ type T = void               │ const T = {                    │
│                                │                             │   type: 'void'                 │
│                                │                             │ }                              │
│                                │                             │                                │
└────────────────────────────────┴─────────────────────────────┴────────────────────────────────┘
```

<a name='types-options'></a>

### Options

You can pass Json Schema options on the last argument of any given type. Option hints specific to each type are provided for convenience.

```typescript
// String must be an email
const T = Type.String({                              // const T = {
  format: 'email'                                    //   type: 'string',
})                                                   //   format: 'email'
                                                     // }

// Number must be a multiple of 2
const T = Type.Number({                              // const T = {
  multipleOf: 2                                      //  type: 'number',
})                                                   //  multipleOf: 2
                                                     // }

// Array must have at least 5 integer values
const T = Type.Array(Type.Integer(), {               // const T = {
  minItems: 5                                        //   type: 'array',
})                                                   //   minItems: 5,
                                                     //   items: {
                                                     //     type: 'integer'
                                                     //   }
                                                     // }
```

<a name='types-properties'></a>

### Properties

Object properties can be modified with Readonly and Optional. The following table shows how these modifiers map between TypeScript and Json Schema.

```typescript
┌────────────────────────────────┬─────────────────────────────┬────────────────────────────────┐
│ TypeBox                        │ TypeScript                  │ Json Schema                    │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Object({        │ type T = {                  │ const T = {                    │
│   name: Type.ReadonlyOptional( │   readonly name?: string    │   type: 'object',              │
│     Type.String()              │ }                           │   properties: {                │
│   )                            │                             │     name: {                    │
│ })  	                         │                             │       type: 'string'           │
│                                │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Object({        │ type T = {                  │ const T = {                    │
│   name: Type.Readonly(         │   readonly name: string     │   type: 'object',              │
│     Type.String()              │ }                           │   properties: {                │
│   )                            │                             │     name: {                    │
│ })  	                         │                             │       type: 'string'           │
│                                │                             │     }                          │
│                                │                             │   },                           │
│                                │                             │   required: ['name']           │
│                                │                             │ }                              │
│                                │                             │                                │
├────────────────────────────────┼─────────────────────────────┼────────────────────────────────┤
│ const T = Type.Object({        │ type T = {                  │ const T = {                    │
│   name: Type.Optional(         │   name?: string             │   type: 'object',              │
│     Type.String()              │ }                           │   properties: {                │
│   )                            │                             │     name: {                    │
│ })  	                         │                             │       type: 'string'           │
│                                │                             │     }                          │
│                                │                             │   }                            │
│                                │                             │ }                              │
│                                │                             │                                │
└────────────────────────────────┴─────────────────────────────┴────────────────────────────────┘
```

<a name='types-generics'></a>

### Generic Types

Generic types can be created with generic functions.

```typescript
const Nullable = <T extends TSchema>(T: T) => {     // type Nullable<T> = T | null
  return Type.Union([T, Type.Null()])
}

const T = Nullable(Type.String())                  // type T = Nullable<string>
```

<a name='types-recursive'></a>

### Recursive Types

Use the Recursive function to create recursive types.

```typescript
const Node = Type.Recursive(This => Type.Object({    // const Node = {
  id: Type.String(),                                 //   $id: 'Node',
  nodes: Type.Array(This)                            //   type: 'object',
}), { $id: 'Node' })                                 //   properties: {
                                                     //     id: {
                                                     //       type: 'string'
                                                     //     },
                                                     //     nodes: {
                                                     //       type: 'array',
                                                     //       items: {
                                                     //         $ref: 'Node'
                                                     //       }
                                                     //     }
                                                     //   },
                                                     //   required: [
                                                     //     'id',
                                                     //     'nodes'
                                                     //   ]
                                                     // }

type Node = Static<typeof Node>                      // type Node = {
                                                     //   id: string
                                                     //   nodes: Node[]
                                                     // }

function test(node: Node) {
  const id = node.nodes[0].nodes[0].id               // id is string
}
```

<a name='types-modules'></a>

### Module Types

Module types are containers for a set of referential types. Modules act as namespaces, enabling types to reference one another via string identifiers. Modules support both singular and mutually recursive references, as well as deferred dereferencing for computed types such as Partial. Types imported from a module are expressed using the Json Schema `$defs` keyword. 

```typescript
const Module = Type.Module({
  PartialUser: Type.Partial(Type.Ref('User')),  // TComputed<'Partial', [TRef<'User'>]>

  User: Type.Object({                           // TObject<{
    id: Type.String(),                          //   user: TString,
    name: Type.String(),                        //   name: TString,
    email: Type.String()                        //   email: TString
  }),                                           // }>
})
const User = Module.Import('User')               // const User: TImport<{...}, 'User'>

type User = Static<typeof User>                  // type User = { 
                                                 //   id: string,
                                                 //   name: string,
                                                 //   email: string
                                                 // }

const PartialUser = Module.Import('PartialUser') // const PartialUser: TImport<{...}, 'PartialUser'>

type PartialUser = Static<typeof PartialUser>    // type PartialUser = { 
                                                 //   id?: string,
                                                 //   name?: string,
                                                 //   email?: string
                                                 // }
```

<a name='types-template-literal'></a>

### Template Literal Types

TypeBox supports template literal types with the TemplateLiteral function. This type can be created using a syntax similar to the TypeScript template literal syntax or composed from exterior types. TypeBox encodes template literals as regular expressions which enables the template to be checked by Json Schema validators. This type also supports regular expression parsing that enables template patterns to be used for generative types. The following shows both TypeScript and TypeBox usage.

```typescript
// TypeScript

type K = `prop${'A'|'B'|'C'}`                        // type T = 'propA' | 'propB' | 'propC'

type R = Record<K, string>                           // type R = {
                                                     //   propA: string
                                                     //   propB: string
                                                     //   propC: string
                                                     // }

// TypeBox

const K = Type.TemplateLiteral('prop${A|B|C}')       // const K: TTemplateLiteral<[
                                                     //   TLiteral<'prop'>,
                                                     //   TUnion<[
                                                     //      TLiteral<'A'>,
                                                     //      TLiteral<'B'>,
                                                     //      TLiteral<'C'>,
                                                     //   ]>
                                                     // ]>

const R = Type.Record(K, Type.String())              // const R: TObject<{
                                                     //   propA: TString,
                                                     //   propB: TString,
                                                     //   propC: TString,
                                                     // }>
```

<a name='types-indexed'></a>

### Indexed Access Types

TypeBox supports indexed access types with the Index function. This function enables uniform access to interior property and element types without having to extract them from the underlying schema representation. Index types are supported for Object, Array, Tuple, Union and Intersect types.

```typescript
const T = Type.Object({                              // type T = {
  x: Type.Number(),                                  //   x: number,
  y: Type.String(),                                  //   y: string,
  z: Type.Boolean()                                  //   z: boolean
})                                                   // }

const A = Type.Index(T, ['x'])                       // type A = T['x']
                                                     //
                                                     // ... evaluated as
                                                     //
                                                     // const A: TNumber

const B = Type.Index(T, ['x', 'y'])                  // type B = T['x' | 'y']
                                                     //
                                                     // ... evaluated as
                                                     //
                                                     // const B: TUnion<[
                                                     //   TNumber,
                                                     //   TString,
                                                     // ]>

const C = Type.Index(T, Type.KeyOf(T))               // type C = T[keyof T]
                                                     //
                                                     // ... evaluated as
                                                     // 
                                                     // const C: TUnion<[
                                                     //   TNumber,
                                                     //   TString,
                                                     //   TBoolean
                                                     // ]>
```

<a name='types-mapped'></a>

### Mapped Types

TypeBox supports mapped types with the Mapped function. This function accepts two arguments, the first is a union type typically derived from KeyOf, the second is a mapping function that receives a mapping key `K` that can be used to index properties of a type. The following implements a mapped type that remaps each property to be `T | null`.

```typescript
const T = Type.Object({                              // type T = {
  x: Type.Number(),                                  //   x: number,
  y: Type.String(),                                  //   y: string,
  z: Type.Boolean()                                  //   z: boolean
})                                                   // }

const M = Type.Mapped(Type.KeyOf(T), K => {          // type M = { [K in keyof T]: T[K] | null }
  return Type.Union([Type.Index(T, K), Type.Null()]) //
})                                                   // ... evaluated as
                                                     // 
                                                     // const M: TObject<{
                                                     //   x: TUnion<[TNumber, TNull]>,
                                                     //   y: TUnion<[TString, TNull]>,
                                                     //   z: TUnion<[TBoolean, TNull]>
                                                     // }>
```

<a name='types-conditional'></a>

### Conditional Types

TypeBox supports runtime conditional types with the Extends function. This function performs a structural assignability check against the first (`left`) and second (`right`) arguments and will return either the third (`true`) or fourth (`false`) argument based on the result. The conditional types Exclude and Extract are also supported. The following shows both TypeScript and TypeBox examples of conditional types.

```typescript
// Extends
const A = Type.Extends(                              // type A = string extends number ? 1 : 2
  Type.String(),                                     //   
  Type.Number(),                                     // ... evaluated as
  Type.Literal(1),                                   //
  Type.Literal(2)                                    // const A: TLiteral<2>
)

// Extract
const B = Type.Extract(                              // type B = Extract<1 | 2 | 3, 1>
  Type.Union([                                       //
    Type.Literal(1),                                 // ... evaluated as
    Type.Literal(2),                                 //
    Type.Literal(3)                                  // const B: TLiteral<1>
  ]), 
  Type.Literal(1)
)

// Exclude
const C = Type.Exclude(                              // type C = Exclude<1 | 2 | 3, 1>
  Type.Union([                                       // 
    Type.Literal(1),                                 // ... evaluated as
    Type.Literal(2),                                 //
    Type.Literal(3)                                  // const C: TUnion<[
  ]),                                                //   TLiteral<2>,
  Type.Literal(1)                                    //   TLiteral<3>,
)                                                    // ]>
```

<a name='types-transform'></a>

### Transform Types

TypeBox supports value decoding and encoding with Transform types. These types work in tandem with the Encode and Decode functions available on the Value and TypeCompiler submodules. Transform types can be used to convert Json encoded values into constructs more natural to JavaScript. The following creates a Transform type to decode numbers into Dates using the Value submodule.

```typescript
import { Value } from '@sinclair/typebox/value'

const T = Type.Transform(Type.Number())
  .Decode(value => new Date(value))                  // decode: number to Date
  .Encode(value => value.getTime())                  // encode: Date to number

const D = Value.Decode(T, 0)                         // const D = Date(1970-01-01T00:00:00.000Z)
const E = Value.Encode(T, D)                         // const E = 0
```
Use the StaticEncode or StaticDecode types to infer a Transform type.
```typescript
import { Static, StaticDecode, StaticEncode } from '@sinclair/typebox'

const T = Type.Transform(Type.Array(Type.Number(), { uniqueItems: true }))         
  .Decode(value => new Set(value))
  .Encode(value => [...value])

type D = StaticDecode<typeof T>                      // type D = Set<number>      
type E = StaticEncode<typeof T>                      // type E = Array<number>
type T = Static<typeof T>                            // type T = Array<number>
```

<a name='types-unsafe'></a>

### Unsafe Types

TypeBox supports user defined types with Unsafe. This type allows you to specify both schema representation and inference type. The following creates an Unsafe type with a number schema that infers as string.

```typescript
const T = Type.Unsafe<string>({ type: 'number' })    // const T = { type: 'number' }

type T = Static<typeof T>                            // type T = string - ?
```
The Unsafe type is often used to create schematics for extended specifications like OpenAPI.
```typescript

const Nullable = <T extends TSchema>(schema: T) => Type.Unsafe<Static<T> | null>({ 
  ...schema, nullable: true 
})

const T = Nullable(Type.String())                    // const T = {
                                                     //   type: 'string',
                                                     //   nullable: true
                                                     // }

type T = Static<typeof T>                            // type T = string | null

const StringEnum = <T extends string[]>(values: [...T]) => Type.Unsafe<T[number]>({ 
  type: 'string', enum: values 
})
const S = StringEnum(['A', 'B', 'C'])                // const S = {
                                                     //   enum: ['A', 'B', 'C']
                                                     // }

type S = Static<typeof T>                            // type S = 'A' | 'B' | 'C'
```
<a name='types-guard'></a>

### TypeGuard

TypeBox can check its own types with the TypeGuard module. This module is written for type introspection and provides structural tests for every built-in TypeBox type. Functions of this module return `is` guards which can be used with control flow assertions to obtain schema inference for unknown values. The following guards that the value `T` is TString.

```typescript
import { TypeGuard, Kind } from '@sinclair/typebox'

const T = { [Kind]: 'String', type: 'string' }

if(TypeGuard.IsString(T)) {

  // T is TString
}
```

<a name='values'></a>

## Values

TypeBox provides an optional Value submodule that can be used to perform structural operations on JavaScript values. This submodule includes functionality to create, check and cast values from types as well as check equality, clone, diff and patch JavaScript values. This submodule is provided via optional import.

```typescript
import { Value } from '@sinclair/typebox/value'
```

<a name='values-assert'></a>

### Assert

Use the Assert function to assert a value is valid.

```typescript
let value: unknown = 1

Value.Assert(Type.Number(), value)                   // throws AssertError if invalid
```

<a name='values-create'></a>

### Create

Use the Create function to create a value from a type. TypeBox will use default values if specified.

```typescript
const T = Type.Object({ x: Type.Number(), y: Type.Number({ default: 42 }) })

const A = Value.Create(T)                            // const A = { x: 0, y: 42 }
```

<a name='values-clone'></a>

### Clone

Use the Clone function to deeply clone a value.

```typescript
const A = Value.Clone({ x: 1, y: 2, z: 3 })          // const A = { x: 1, y: 2, z: 3 }
```

<a name='values-check'></a>

### Check

Use the Check function to type check a value.

```typescript
const T = Type.Object({ x: Type.Number() })

const R = Value.Check(T, { x: 1 })                   // const R = true
```

<a name='values-convert'></a>

### Convert

Use the Convert function to convert a value into its target type if a reasonable conversion is possible. This function may return an invalid value and should be checked before use. Its return type is `unknown`.

```typescript
const T = Type.Object({ x: Type.Number() })

const R1 = Value.Convert(T, { x: '3.14' })           // const R1 = { x: 3.14 }

const R2 = Value.Convert(T, { x: 'not a number' })   // const R2 = { x: 'not a number' }
```

<a name='values-clean'></a>

### Clean

Use Clean to remove excess properties from a value. This function does not check the value and returns an unknown type. You should Check the result before use. Clean is a mutable operation. To avoid mutation, Clone the value first.

```typescript
const T = Type.Object({ 
  x: Type.Number(), 
  y: Type.Number() 
})

const X = Value.Clean(T, null)                        // const 'X = null

const Y = Value.Clean(T, { x: 1 })                    // const 'Y = { x: 1 }

const Z = Value.Clean(T, { x: 1, y: 2, z: 3 })        // const 'Z = { x: 1, y: 2 }
```

<a name='values-default'></a>

### Default

Use Default to generate missing properties on a value using default schema annotations if available. This function does not check the value and returns an unknown type. You should Check the result before use. Default is a mutable operation. To avoid mutation, Clone the value first.

```typescript
const T = Type.Object({ 
  x: Type.Number({ default: 0 }), 
  y: Type.Number({ default: 0 })
})

const X = Value.Default(T, null)                        // const 'X = null - non-enumerable

const Y = Value.Default(T, { })                         // const 'Y = { x: 0, y: 0 }

const Z = Value.Default(T, { x: 1 })                    // const 'Z = { x: 1, y: 0 }
```

<a name='values-cast'></a>

### Cast

Use the Cast function to upcast a value into a target type. This function will retain as much information as possible from the original value. The Cast function is intended to be used in data migration scenarios where existing values need to be upgraded to match a modified type.

```typescript
const T = Type.Object({ x: Type.Number(), y: Type.Number() }, { additionalProperties: false })

const X = Value.Cast(T, null)                        // const X = { x: 0, y: 0 }

const Y = Value.Cast(T, { x: 1 })                    // const Y = { x: 1, y: 0 }

const Z = Value.Cast(T, { x: 1, y: 2, z: 3 })        // const Z = { x: 1, y: 2 }
```

<a name='values-decode'></a>

### Decode

Use the Decode function to decode a value from a type or throw if the value is invalid. The return value will infer as the decoded type. This function will run Transform codecs if available.

```typescript
const A = Value.Decode(Type.String(), 'hello')        // const A = 'hello'

const B = Value.Decode(Type.String(), 42)             // throw
```
<a name='values-decode'></a>

### Encode

Use the Encode function to encode a value to a type or throw if the value is invalid. The return value will infer as the encoded type. This function will run Transform codecs if available.

```typescript
const A = Value.Encode(Type.String(), 'hello')        // const A = 'hello'

const B = Value.Encode(Type.String(), 42)             // throw
```

<a name='values-parse'></a>

### Parse

Use the Parse function to parse a value. This function calls the `Clone` `Clean`, `Default`, `Convert`, `Assert` and `Decode` Value functions in this exact order to process a value.

```typescript
const R = Value.Parse(Type.String(), 'hello')      // const R: string = "hello"

const E = Value.Parse(Type.String(), undefined)    // throws AssertError 
```

You can override the order in which functions are run, or omit functions entirely using the following.

```typescript
// Runs no functions.

const R = Value.Parse([], Type.String(), 12345)

// Runs the Assert() function.

const E = Value.Parse(['Assert'], Type.String(), 12345)

// Runs the Convert() function followed by the Assert() function.

const S = Value.Parse(['Convert', 'Assert'], Type.String(), 12345)
```

<a name='values-equal'></a>

### Equal

Use the Equal function to deeply check for value equality.

```typescript
const R = Value.Equal(                               // const R = true
  { x: 1, y: 2, z: 3 },
  { x: 1, y: 2, z: 3 }
)
```

<a name='values-hash'></a>

### Hash

Use the Hash function to create a [FNV1A-64](https://en.wikipedia.org/wiki/Fowler%E2%80%93Noll%E2%80%93Vo_hash_function) non-cryptographic hash of a value.

```typescript
const A = Value.Hash({ x: 1, y: 2, z: 3 })           // const A = 2910466848807138541n

const B = Value.Hash({ x: 1, y: 4, z: 3 })           // const B = 1418369778807423581n
```

<a name='values-diff'></a>

### Diff

Use the Diff function to generate a sequence of edits that will transform one value into another.

```typescript
const E = Value.Diff(                                // const E = [
  { x: 1, y: 2, z: 3 },                              //   { type: 'update', path: '/y', value: 4 },
  { y: 4, z: 5, w: 6 }                               //   { type: 'update', path: '/z', value: 5 },
)                                                    //   { type: 'insert', path: '/w', value: 6 },
                                                     //   { type: 'delete', path: '/x' }
                                                     // ]
```

<a name='values-patch'></a>

### Patch

Use the Patch function to apply a sequence of edits.

```typescript
const A = { x: 1, y: 2 }

const B = { x: 3 }

const E = Value.Diff(A, B)                           // const E = [
                                                     //   { type: 'update', path: '/x', value: 3 },
                                                     //   { type: 'delete', path: '/y' }
                                                     // ]

const C = Value.Patch<typeof B>(A, E)                // const C = { x: 3 }
```

<a name='values-errors'></a>

### Errors

Use the Errors function to enumerate validation errors.

```typescript
const T = Type.Object({ x: Type.Number(), y: Type.Number() })

const R = [...Value.Errors(T, { x: '42' })]          // const R = [{
                                                     //   schema: { type: 'number' },
                                                     //   path: '/x',
                                                     //   value: '42',
                                                     //   message: 'Expected number'
                                                     // }, {
                                                     //   schema: { type: 'number' },
                                                     //   path: '/y',
                                                     //   value: undefined,
                                                     //   message: 'Expected number'
                                                     // }]
```

<a name='values-mutate'></a>

### Mutate

Use the Mutate function to perform a deep mutable value assignment while retaining internal references.

```typescript
const Y = { z: 1 }                                   // const Y = { z: 1 }
const X = { y: Y }                                   // const X = { y: { z: 1 } }
const A = { x: X }                                   // const A = { x: { y: { z: 1 } } }

Value.Mutate(A, { x: { y: { z: 2 } } })              // A' = { x: { y: { z: 2 } } }

const R0 = A.x.y.z === 2                             // const R0 = true
const R1 = A.x.y === Y                               // const R1 = true
const R2 = A.x === X                                 // const R2 = true
```

<a name='values-pointer'></a>

### Pointer

Use ValuePointer to perform mutable updates on existing values using [RFC6901](https://www.rfc-editor.org/rfc/rfc6901) Json Pointers.

```typescript
import { ValuePointer } from '@sinclair/typebox/value'

const A = { x: 0, y: 0, z: 0 }

ValuePointer.Set(A, '/x', 1)                         // A' = { x: 1, y: 0, z: 0 }
ValuePointer.Set(A, '/y', 1)                         // A' = { x: 1, y: 1, z: 0 }
ValuePointer.Set(A, '/z', 1)                         // A' = { x: 1, y: 1, z: 1 }
```



<a name='syntax'></a>

## Syntax Types

TypeBox provides experimental support for parsing TypeScript annotation syntax into TypeBox types.

This feature is provided via optional import.

```typescript
import { Syntax } from '@sinclair/typebox/syntax'
```

<a name='syntax-create'></a>

### Create

Use the Syntax function to create TypeBox types from TypeScript syntax ([Example](https://www.typescriptlang.org/play/?moduleResolution=99&module=199&ts=5.8.0-beta#code/JYWwDg9gTgLgBAbzgZQJ4DsYEMAecC+cAZlBCHAOQACAzsOgMYA2WwUA9DKmAKYBGEHOxoZsOCgChQkWIhTYYwBgWKly1OoxZtO3foMkSGEdDXgAVOAF4Uo3AAoABkhwAuOOgCuIPjygAaOFR3Lx8-AkcASjgY2Jj2djhjUwt3cwB5PgArHgYYAB4ECTiS0rLyisrYhNi3OHMAOW9fAOKq9o7OuBqY4PqmsKg2rpHR+MT8AD4JCS5eeut5LEUGfLmeCCJ6ybHKmvWFmyLdk86euDrQlv9h07uy876rv1v7t-GCIA))

```typescript
const T = Syntax(`{ x: number, y: number }`)        // const T: TObject<{
                                                    //   x: TNumber,
                                                    //   y: TNumber
                                                    // }>

type T = Static<typeof T>                            // type T = {
                                                     //   x: number,
                                                     //   y: number
                                                     // }
```

<a name="syntax-parameters"></a>

### Parameters

Syntax types can be parameterized to receive exterior types ([Example](https://www.typescriptlang.org/play/?moduleResolution=99&module=199&ts=5.8.0-beta#code/JYWwDg9gTgLgBAbzgZQJ4DsYEMAecC+cAZlBCHAOQACAzsOgMYA2WwUA9DKmAKYBGEHOxoZsOCgCgJDCOhrwAKnAC8KUbgAUAAyQ4AXHHQBXEHx5QANHFQHjp8wS0BKOK7ev27ODLmKDCgHk+ACseBhgAHgQJd1i4+ITEpLdPN304BQA5EzNLGOSCwqK4VNcbDOz7KHzi2rqPL3wAPikfeRQVNUxNJCV8Ky0ABSxYYCwmCIUm52LUtvhkfyDQ8Kia+o2C0rh0wLAYYFlxycrcpot1zav47fK9g6OJrJzzFuv3m8amoA))

```typescript
const T = Syntax(`{ x: number, y: number }`)        // const T: TObject<{
                                                    //   x: TNumber,
                                                    //   y: TNumber
                                                    // }>

const S = Syntax({ T }, `Partial<T>`)               // const S: TObject<{
                                                    //   x: TOptional<TNumber>,
                                                    //   y: TOptional<TNumber>
                                                    // }>
```



<a name="syntax-generics"></a>

### Generics

Syntax types support generic parameters in the following way ([Example](https://www.typescriptlang.org/play/?moduleResolution=99&module=199&ts=5.8.0-beta#code/JYWwDg9gTgLgBAbzgZQJ4DsYEMAecC+cAZlBCHAOQACAzsOgMYA2WwUA9DKmAKYBGEHOxoZsOCgChQkWIhTYYwBgWKly1OoxZtO3foMkSGEdDXgA1HgxjQ4AXhSjcACgAGAHgAaAGjgBNXwAtAD45CTg4HAAuOB84cLhUGID4iIAvGMD4-FcASgkjEzM4ACEsOhpLa2gae0dMFyQqmygCX1cEBOi4Zuh3AEZfAAZh4O8EpJ6rFvcRuEG4IbGEjKnqqFnh337lnPyJLl5S8uBK6Zq65AUld0OeCCJjit6oGlCIiPZ2ODun05fag5Oh8QaCweCIZCoV8Pt0kN0FpM5qshm0ElCMZisSCYRFJvCYnNJgsUWjseSKeDcXBVgTFr4kb5Vv0COjKezsTD8EA))

```typescript
const Vector = Syntax(`<X, Y, Z> { 
  x: X, 
  y: Y, 
  z: Z 
}`)

const BasisVectors = Syntax({ Vector }, `{
  x: Vector<1, 0, 0>,
  y: Vector<0, 1, 0>,
  z: Vector<0, 0, 1>,
}`)

type BasisVectors = Static<typeof BasisVectors>     // type BasisVectors = {
                                                    //   x: { x: 1, y: 0, z: 0 },
                                                    //   y: { x: 0, y: 1, z: 0 },
                                                    //   z: { x: 0, y: 0, z: 1 }
                                                    // }
```

<a name='syntax-options'></a>

### Options

Options can be passed via the last parameter.

```typescript
const T = Syntax(`number`, { minimum: 42 })       // const T = {
                                                  //   type: 'number',
                                                  //   minimum: 42
                                                  // }
```

<a name='syntax-no-infer'></a>

### NoInfer

Syntax parsing is an expensive type level operation and can impact on language service performance. Use the NoInfer function parse syntax at runtime only.

```typescript
import { NoInfer } from '@sinclair/typebox/syntax'

const T = NoInfer(`number | string`)                // const T: TSchema = {
                                                    //   anyOf: [
                                                    //     { type: 'number' },
                                                    //     { type: 'string' }
                                                    //   ]
                                                    // }
```

<a name='typeregistry'></a>

## TypeRegistry

The TypeBox type system can be extended with additional types and formats using the TypeRegistry and FormatRegistry modules. These modules integrate deeply with TypeBox's internal type checking infrastructure and can be used to create application specific types, or register schematics for alternative specifications.

<a name='typeregistry-type'></a>

### TypeRegistry

Use the TypeRegistry to register a type. The Kind must match the registered type name.

```typescript
import { TSchema, Kind, TypeRegistry } from '@sinclair/typebox'

TypeRegistry.Set('Foo', (schema, value) => value === 'foo')

const Foo = { [Kind]: 'Foo' } as TSchema 

const A = Value.Check(Foo, 'foo')                    // const A = true

const B = Value.Check(Foo, 'bar')                    // const B = false
```

<a name='typeregistry-format'></a>

### FormatRegistry

Use the FormatRegistry to register a string format.

```typescript
import { FormatRegistry } from '@sinclair/typebox'

FormatRegistry.Set('foo', (value) => value === 'foo')

const T = Type.String({ format: 'foo' })

const A = Value.Check(T, 'foo')                      // const A = true

const B = Value.Check(T, 'bar')                      // const B = false
```

<a name='typecheck'></a>

## TypeCheck

TypeBox types target Json Schema Draft 7 and are compatible with any validator that supports this specification. TypeBox also provides a built-in type checking compiler designed specifically for TypeBox types that offers high performance compilation and value checking.

The following sections detail using Ajv and the TypeBox compiler infrastructure.

<a name='typecheck-ajv'></a>

## Ajv

The following shows the recommended setup for Ajv.

```bash
$ npm install ajv ajv-formats --save
```

```typescript
import { Type }   from '@sinclair/typebox'
import addFormats from 'ajv-formats'
import Ajv        from 'ajv'

const ajv = addFormats(new Ajv({}), [
  'date-time',
  'time',
  'date',
  'email',
  'hostname',
  'ipv4',
  'ipv6',
  'uri',
  'uri-reference',
  'uuid',
  'uri-template',
  'json-pointer',
  'relative-json-pointer',
  'regex'
])

const validate = ajv.compile(Type.Object({
  x: Type.Number(),
  y: Type.Number(),
  z: Type.Number()
}))

const R = validate({ x: 1, y: 2, z: 3 })             // const R = true
```

<a name='typecheck-typecompiler'></a>

### TypeCompiler

The TypeBox TypeCompiler is a high performance JIT validation compiler that transforms TypeBox types into optimized JavaScript validation routines. The compiler is tuned for fast compilation as well as fast value assertion. It is built to serve as a validation backend that can be integrated into larger applications. It can also be used for code generation.

The TypeCompiler is provided as an optional import.

```typescript
import { TypeCompiler } from '@sinclair/typebox/compiler'
```

Use the Compile function to JIT compile a type. Note that compilation is generally an expensive operation and should only be performed once per type during application start up. TypeBox does not cache previously compiled types, and applications are expected to hold references to each compiled type for the lifetime of the application.

```typescript
const C = TypeCompiler.Compile(Type.Object({         // const C: TypeCheck<TObject<{
  x: Type.Number(),                                  //     x: TNumber;
  y: Type.Number(),                                  //     y: TNumber;
  z: Type.Number()                                   //     z: TNumber;
}))                                                  // }>>

const R = C.Check({ x: 1, y: 2, z: 3 })              // const R = true
```

Use the Errors function to generate diagnostic errors for a value. The Errors function will return an iterator that when enumerated; will perform an exhaustive check across the entire value yielding any error found. For performance, this function should only be called after a failed Check. Applications may also choose to yield only the first value to avoid exhaustive error generation.

```typescript
const C = TypeCompiler.Compile(Type.Object({         // const C: TypeCheck<TObject<{
  x: Type.Number(),                                  //     x: TNumber;
  y: Type.Number(),                                  //     y: TNumber;
  z: Type.Number()                                   //     z: TNumber;
}))                                                  // }>>

const value = { }

const first = C.Errors(value).First()                // const first = {
                                                     //   schema: { type: 'number' },
                                                     //   path: '/x',
                                                     //   value: undefined,
                                                     //   message: 'Expected number'
                                                     // }

const all = [...C.Errors(value)]                     // const all = [{
                                                     //   schema: { type: 'number' },
                                                     //   path: '/x',
                                                     //   value: undefined,
                                                     //   message: 'Expected number'
                                                     // }, {
                                                     //   schema: { type: 'number' },
                                                     //   path: '/y',
                                                     //   value: undefined,
                                                     //   message: 'Expected number'
                                                     // }, {
                                                     //   schema: { type: 'number' },
                                                     //   path: '/z',
                                                     //   value: undefined,
                                                     //   message: 'Expected number'
                                                     // }]
```

Use the Code function to generate assertion functions as strings. This function can be used to generate code that can be written to disk as importable modules. This technique is sometimes referred to as Ahead of Time (AOT) compilation. The following generates code to check a string.

```typescript
const C = TypeCompiler.Code(Type.String())           // const C = `return function check(value) {
                                                     //   return (
                                                     //     (typeof value === 'string')
                                                     //   )
                                                     // }`
```

<a name='typemap'></a>

## TypeMap

TypeBox offers an external package for bidirectional mapping between TypeBox, Valibot, and Zod type libraries. It also includes syntax parsing support for Valibot and Zod and supports the Standard Schema specification. For more details on TypeMap, refer to the project repository.

[TypeMap Repository](https://github.com/sinclairzx81/typemap)

<a name='typemap-usage'></a>

### Usage

TypeMap needs to be installed separately

```bash
$ npm install @sinclair/typemap
```

Once installed it offers advanced structural remapping between various runtime type libraries ([Example](https://www.typescriptlang.org/play/?moduleResolution=99&module=199&ts=5.8.0-beta#code/JYWwDg9gTgLgBAbzgFQJ5gKYCEIA8A0cAyqgHYwCGBcAWhACZwC+cAZlBCHAOQACAzsFIBjADYVgUAPQx0GEBTDcAUMuERS-eMjgBeFHJy4AFAAMkuAFxxSAVxAAjDFEKprdx88IAvd-adQzKYAlHBwUlJw6pra1sgA8g4AVhjCMAA8CMphObl5+QWFRcW5ETlWKABy-s4A3NkljU3NBWVhblU1UPUtvX3FbXC+nZ7dDf0TE2VMAHyq0VrEesRklCbIoS1lC-BE1twWfqOuRwE+p87MKmoaiwBKy3T0xkTBAHRgFFD8GMZ2oqJNnltrd4HdrFlJltImEKh4Aj0oU1Bh14XVxkiBjChhcxpjGtMwkA))

```typescript
import { TypeBox, Syntax, Zod } from '@sinclair/typemap'

const T = TypeBox(`{ x: number, y: number, z: number }`)  // const T: TObject<{
                                                          //   x: TNumber;
                                                          //   y: TNumber;
                                                          //   z: TNumber;
                                                          // }>

const S = Syntax(T)                // const S: '{ x: number, y: number, z: number }'

const R = Zod(S).parse(null)       // const R: {
                                   //   x: number;
                                   //   y: number;
                                   //   z: number;
                                   // }                    
```

<a name='typesystem'></a>

## TypeSystem

The TypeBox TypeSystem module provides configurations to use either Json Schema or TypeScript type checking semantics. Configurations made to the TypeSystem module are observed by the TypeCompiler, Value and Error modules.

<a name='typesystem-policies'></a>

### Policies

TypeBox validates using standard Json Schema assertion policies by default. The TypeSystemPolicy module can override some of these to have TypeBox assert values inline with TypeScript static checks. It also provides overrides for certain checking rules related to non-serializable values (such as void) which can be helpful in Json based protocols such as Json Rpc 2.0. 

The following overrides are available.

```typescript
import { TypeSystemPolicy } from '@sinclair/typebox/system'

// Disallow undefined values for optional properties (default is false)
//
// const A: { x?: number } = { x: undefined } - disallowed when enabled

TypeSystemPolicy.ExactOptionalPropertyTypes = true

// Allow arrays to validate as object types (default is false)
//
// const A: {} = [] - allowed in TS

TypeSystemPolicy.AllowArrayObject = true

// Allow numeric values to be NaN or + or - Infinity (default is false)
//
// const A: number = NaN - allowed in TS

TypeSystemPolicy.AllowNaN = true

// Allow void types to check with undefined and null (default is false)
//
// Used to signal void return on Json-Rpc 2.0 protocol

TypeSystemPolicy.AllowNullVoid = true
```

<a name='error-function'></a>

## Error Function

Error messages in TypeBox can be customized by defining an ErrorFunction. This function allows for the localization of error messages as well as enabling custom error messages for custom types. By default, TypeBox will generate messages using the `en-US` locale. To support additional locales, you can replicate the function found in `src/errors/function.ts` and create a locale specific translation. The function can then be set via SetErrorFunction.

The following example shows an inline error function that intercepts errors for String, Number and Boolean only. The DefaultErrorFunction is used to return a default error message.


```typescript
import { SetErrorFunction, DefaultErrorFunction, ValueErrorType } from '@sinclair/typebox/errors'

SetErrorFunction((error) => { // i18n override
  switch(error.errorType) {
    /* en-US */ case ValueErrorType.String: return 'Expected string'
    /* fr-FR */ case ValueErrorType.Number: return 'Nombre attendu'  
    /* ko-KR */ case ValueErrorType.Boolean: return '예상 부울'      
    /* en-US */ default: return DefaultErrorFunction(error)          
  }
})
const T = Type.Object({                              // const T: TObject<{
  x: Type.String(),                                  //  TString,
  y: Type.Number(),                                  //  TNumber,
  z: Type.Boolean()                                  //  TBoolean
})                                                   // }>

const E = [...Value.Errors(T, {                      // const E = [{
  x: null,                                           //   type: 48,
  y: null,                                           //   schema: { ... },
  z: null                                            //   path: '/x',
})]                                                  //   value: null,
                                                     //   message: 'Expected string'
                                                     // }, {
                                                     //   type: 34,
                                                     //   schema: { ... },
                                                     //   path: '/y',
                                                     //   value: null,
                                                     //   message: 'Nombre attendu'
                                                     // }, {
                                                     //   type: 14,
                                                     //   schema: { ... },
                                                     //   path: '/z',
                                                     //   value: null,
                                                     //   message: '예상 부울'
                                                     // }]
```

<a name='workbench'></a>

## TypeBox Workbench

TypeBox offers a web based code generation tool that can convert TypeScript types into TypeBox types as well as several other ecosystem libraries.

[TypeBox Workbench Link Here](https://sinclairzx81.github.io/typebox-workbench/)

<a name='codegen'></a>

## TypeBox Codegen

TypeBox provides a code generation library that can be integrated into toolchains to automate type translation between TypeScript and TypeBox. This library also includes functionality to transform TypeScript types to other ecosystem libraries.

[TypeBox Codegen Link Here](https://github.com/sinclairzx81/typebox-codegen)

<a name='ecosystem'></a>

## Ecosystem

The following is a list of community packages that offer general tooling, extended functionality and framework integration support for TypeBox.

| Package   |  Description |
| ------------- | ------------- |
| [drizzle-typebox](https://www.npmjs.com/package/drizzle-typebox) | Generates TypeBox types from Drizzle ORM schemas |
| [elysia](https://github.com/elysiajs/elysia) | Fast and friendly Bun web framework |
| [fastify-type-provider-typebox](https://github.com/fastify/fastify-type-provider-typebox) | Fastify TypeBox integration with the Fastify Type Provider |
| [feathersjs](https://github.com/feathersjs/feathers) | The API and real-time application framework |
| [fetch-typebox](https://github.com/erfanium/fetch-typebox) | Drop-in replacement for fetch that brings easy integration with TypeBox |
| [@lonli-lokli/fetcher-typebox](https://github.com/Lonli-Lokli/fetcher-ts/tree/master/packages/fetcher-typebox) | A strongly-typed fetch wrapper for TypeScript applications with optional runtime validation using TypeBox |
| [h3-typebox](https://github.com/kevinmarrec/h3-typebox) | Schema validation utilities for h3 using TypeBox & Ajv |
| [http-wizard](https://github.com/flodlc/http-wizard) | Type safe http client library for Fastify |
| [json2typebox](https://github.com/hacxy/json2typebox) | Creating TypeBox code from Json Data |
| [nominal-typebox](https://github.com/Coder-Spirit/nominal/tree/main/%40coderspirit/nominal-typebox) | Allows devs to integrate nominal types into TypeBox schemas |
| [openapi-box](https://github.com/geut/openapi-box) | Generate TypeBox types from OpenApi IDL + Http client library |
| [prismabox](https://github.com/m1212e/prismabox) | Converts a prisma.schema to TypeBox schema matching the database models |
| [schema2typebox](https://github.com/xddq/schema2typebox)  | Creating TypeBox code from Json Schemas |
| [sveltekit-superforms](https://github.com/ciscoheat/sveltekit-superforms)  | A comprehensive SvelteKit form library for server and client validation |
| [ts2typebox](https://github.com/xddq/ts2typebox) | Creating TypeBox code from Typescript types |
| [typebox-cli](https://github.com/gsuess/typebox-cli) | Generate Schema with TypeBox from the CLI |
| [typebox-form-parser](https://github.com/jtlapp/typebox-form-parser) | Parses form and query data based on TypeBox schemas |
| [typebox-schema-faker](https://github.com/iam-medvedev/typebox-schema-faker) | Generate fake data from TypeBox schemas for testing, prototyping and development |


<a name='benchmark'></a>

## Benchmark

This project maintains a set of benchmarks that measure Ajv, Value and TypeCompiler compilation and validation performance. These benchmarks can be run locally by cloning this repository and running `npm run benchmark`. The results below show for Ajv version 8.12.0 running on Node 20.10.0.

For additional comparative benchmarks, please refer to [typescript-runtime-type-benchmarks](https://moltar.github.io/typescript-runtime-type-benchmarks/).

<a name='benchmark-compile'></a>

### Compile

This benchmark measures compilation performance for varying types.

```typescript
┌────────────────────────────┬────────────┬──────────────┬──────────────┬──────────────┐
│ (index)                    │ Iterations │ Ajv          │ TypeCompiler │ Performance  │
├────────────────────────────┼────────────┼──────────────┼──────────────┼──────────────┤
│ Literal_String             │ 1000       │ '    211 ms' │ '      8 ms' │ '   26.38 x' │
│ Literal_Number             │ 1000       │ '    185 ms' │ '      5 ms' │ '   37.00 x' │
│ Literal_Boolean            │ 1000       │ '    195 ms' │ '      4 ms' │ '   48.75 x' │
│ Primitive_Number           │ 1000       │ '    149 ms' │ '      7 ms' │ '   21.29 x' │
│ Primitive_String           │ 1000       │ '    135 ms' │ '      5 ms' │ '   27.00 x' │
│ Primitive_String_Pattern   │ 1000       │ '    193 ms' │ '     10 ms' │ '   19.30 x' │
│ Primitive_Boolean          │ 1000       │ '    152 ms' │ '      4 ms' │ '   38.00 x' │
│ Primitive_Null             │ 1000       │ '    147 ms' │ '      4 ms' │ '   36.75 x' │
│ Object_Unconstrained       │ 1000       │ '   1065 ms' │ '     26 ms' │ '   40.96 x' │
│ Object_Constrained         │ 1000       │ '   1183 ms' │ '     26 ms' │ '   45.50 x' │
│ Object_Vector3             │ 1000       │ '    407 ms' │ '      9 ms' │ '   45.22 x' │
│ Object_Box3D               │ 1000       │ '   1777 ms' │ '     24 ms' │ '   74.04 x' │
│ Tuple_Primitive            │ 1000       │ '    485 ms' │ '     11 ms' │ '   44.09 x' │
│ Tuple_Object               │ 1000       │ '   1344 ms' │ '     17 ms' │ '   79.06 x' │
│ Composite_Intersect        │ 1000       │ '    606 ms' │ '     14 ms' │ '   43.29 x' │
│ Composite_Union            │ 1000       │ '    522 ms' │ '     17 ms' │ '   30.71 x' │
│ Math_Vector4               │ 1000       │ '    851 ms' │ '      9 ms' │ '   94.56 x' │
│ Math_Matrix4               │ 1000       │ '    406 ms' │ '     10 ms' │ '   40.60 x' │
│ Array_Primitive_Number     │ 1000       │ '    367 ms' │ '      6 ms' │ '   61.17 x' │
│ Array_Primitive_String     │ 1000       │ '    339 ms' │ '      7 ms' │ '   48.43 x' │
│ Array_Primitive_Boolean    │ 1000       │ '    325 ms' │ '      5 ms' │ '   65.00 x' │
│ Array_Object_Unconstrained │ 1000       │ '   1863 ms' │ '     21 ms' │ '   88.71 x' │
│ Array_Object_Constrained   │ 1000       │ '   1535 ms' │ '     18 ms' │ '   85.28 x' │
│ Array_Tuple_Primitive      │ 1000       │ '    829 ms' │ '     14 ms' │ '   59.21 x' │
│ Array_Tuple_Object         │ 1000       │ '   1674 ms' │ '     14 ms' │ '  119.57 x' │
│ Array_Composite_Intersect  │ 1000       │ '    789 ms' │ '     13 ms' │ '   60.69 x' │
│ Array_Composite_Union      │ 1000       │ '    822 ms' │ '     15 ms' │ '   54.80 x' │
│ Array_Math_Vector4         │ 1000       │ '   1129 ms' │ '     14 ms' │ '   80.64 x' │
│ Array_Math_Matrix4         │ 1000       │ '    673 ms' │ '      9 ms' │ '   74.78 x' │
└────────────────────────────┴────────────┴──────────────┴──────────────┴──────────────┘
```

<a name='benchmark-validate'></a>

### Validate

This benchmark measures validation performance for varying types.

```typescript
┌────────────────────────────┬────────────┬──────────────┬──────────────┬──────────────┬──────────────┐
│ (index)                    │ Iterations │ ValueCheck   │ Ajv          │ TypeCompiler │ Performance  │
├────────────────────────────┼────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ Literal_String             │ 1000000    │ '     17 ms' │ '      5 ms' │ '      5 ms' │ '    1.00 x' │
│ Literal_Number             │ 1000000    │ '     14 ms' │ '     18 ms' │ '      9 ms' │ '    2.00 x' │
│ Literal_Boolean            │ 1000000    │ '     14 ms' │ '     20 ms' │ '      9 ms' │ '    2.22 x' │
│ Primitive_Number           │ 1000000    │ '     17 ms' │ '     19 ms' │ '      9 ms' │ '    2.11 x' │
│ Primitive_String           │ 1000000    │ '     17 ms' │ '     18 ms' │ '     10 ms' │ '    1.80 x' │
│ Primitive_String_Pattern   │ 1000000    │ '    172 ms' │ '     46 ms' │ '     41 ms' │ '    1.12 x' │
│ Primitive_Boolean          │ 1000000    │ '     14 ms' │ '     19 ms' │ '     10 ms' │ '    1.90 x' │
│ Primitive_Null             │ 1000000    │ '     16 ms' │ '     19 ms' │ '      9 ms' │ '    2.11 x' │
│ Object_Unconstrained       │ 1000000    │ '    437 ms' │ '     28 ms' │ '     14 ms' │ '    2.00 x' │
│ Object_Constrained         │ 1000000    │ '    653 ms' │ '     46 ms' │ '     37 ms' │ '    1.24 x' │
│ Object_Vector3             │ 1000000    │ '    201 ms' │ '     22 ms' │ '     12 ms' │ '    1.83 x' │
│ Object_Box3D               │ 1000000    │ '    961 ms' │ '     37 ms' │ '     19 ms' │ '    1.95 x' │
│ Object_Recursive           │ 1000000    │ '   3715 ms' │ '    363 ms' │ '    174 ms' │ '    2.09 x' │
│ Tuple_Primitive            │ 1000000    │ '    107 ms' │ '     23 ms' │ '     11 ms' │ '    2.09 x' │
│ Tuple_Object               │ 1000000    │ '    375 ms' │ '     28 ms' │ '     15 ms' │ '    1.87 x' │
│ Composite_Intersect        │ 1000000    │ '    377 ms' │ '     22 ms' │ '     12 ms' │ '    1.83 x' │
│ Composite_Union            │ 1000000    │ '    337 ms' │ '     30 ms' │ '     17 ms' │ '    1.76 x' │
│ Math_Vector4               │ 1000000    │ '    137 ms' │ '     23 ms' │ '     11 ms' │ '    2.09 x' │
│ Math_Matrix4               │ 1000000    │ '    576 ms' │ '     37 ms' │ '     28 ms' │ '    1.32 x' │
│ Array_Primitive_Number     │ 1000000    │ '    145 ms' │ '     23 ms' │ '     12 ms' │ '    1.92 x' │
│ Array_Primitive_String     │ 1000000    │ '    152 ms' │ '     22 ms' │ '     13 ms' │ '    1.69 x' │
│ Array_Primitive_Boolean    │ 1000000    │ '    131 ms' │ '     20 ms' │ '     13 ms' │ '    1.54 x' │
│ Array_Object_Unconstrained │ 1000000    │ '   2821 ms' │ '     62 ms' │ '     45 ms' │ '    1.38 x' │
│ Array_Object_Constrained   │ 1000000    │ '   2958 ms' │ '    119 ms' │ '    134 ms' │ '    0.89 x' │
│ Array_Object_Recursive     │ 1000000    │ '  14695 ms' │ '   1621 ms' │ '    635 ms' │ '    2.55 x' │
│ Array_Tuple_Primitive      │ 1000000    │ '    478 ms' │ '     35 ms' │ '     28 ms' │ '    1.25 x' │
│ Array_Tuple_Object         │ 1000000    │ '   1623 ms' │ '     63 ms' │ '     48 ms' │ '    1.31 x' │
│ Array_Composite_Intersect  │ 1000000    │ '   1582 ms' │ '     43 ms' │ '     30 ms' │ '    1.43 x' │
│ Array_Composite_Union      │ 1000000    │ '   1331 ms' │ '     76 ms' │ '     40 ms' │ '    1.90 x' │
│ Array_Math_Vector4         │ 1000000    │ '    564 ms' │ '     38 ms' │ '     24 ms' │ '    1.58 x' │
│ Array_Math_Matrix4         │ 1000000    │ '   2382 ms' │ '    111 ms' │ '     83 ms' │ '    1.34 x' │
└────────────────────────────┴────────────┴──────────────┴──────────────┴──────────────┴──────────────┘
```

<a name='benchmark-compression'></a>

### Compression

The following table lists esbuild compiled and minified sizes for each TypeBox module.

```typescript
┌──────────────────────┬────────────┬────────────┬─────────────┐
│ (index)              │ Compiled   │ Minified   │ Compression │
├──────────────────────┼────────────┼────────────┼─────────────┤
│ typebox/compiler     │ '122.4 kb' │ ' 53.4 kb' │ '2.29 x'    │
│ typebox/errors       │ ' 67.6 kb' │ ' 29.6 kb' │ '2.28 x'    │
│ typebox/syntax       │ '132.9 kb' │ ' 54.2 kb' │ '2.45 x'    │
│ typebox/system       │ '  7.4 kb' │ '  3.2 kb' │ '2.33 x'    │
│ typebox/value        │ '150.1 kb' │ ' 62.2 kb' │ '2.41 x'    │
│ typebox              │ '106.8 kb' │ ' 43.2 kb' │ '2.47 x'    │
└──────────────────────┴────────────┴────────────┴─────────────┘
```

<a name='contribute'></a>

## Contribute

TypeBox is open to community contribution. Please ensure you submit an open issue before submitting your pull request. The TypeBox project prefers open community discussion before accepting new features.

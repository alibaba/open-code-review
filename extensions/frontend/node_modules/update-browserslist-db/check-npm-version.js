let { execSync } = require('child_process')
let { existsSync } = require('fs')
let pico = require('picocolors')

if (!existsSync('deno.lock')) {
  try {
    let version = parseInt(execSync('npm -v'))
    if (version <= 6) {
      process.stderr.write(
        pico.red(
          'Update npm or call ' +
            pico.yellow('npx browserslist@latest --update-db') +
            '\n'
        )
      )
      process.exit(1)
    }
  } catch {}
}

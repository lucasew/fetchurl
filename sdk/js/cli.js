#!/usr/bin/env node

import { parseArgs } from 'node:util';
import fs from 'node:fs';
import { fetchurl, parseFetchurlServer } from './fetchurl.js';

const { values, positionals } = parseArgs({
  allowPositionals: true,
  options: {
    url: {
      type: 'string',
      multiple: true,
    },
    output: {
      type: 'string',
      short: 'o',
    },
    help: {
      type: 'boolean',
      short: 'h',
    },
  },
});

if (values.help) {
  console.log('Usage: node cli.js <algo> <hash> --url <url>... -o <output>');
  process.exit(0);
}

if (positionals.length < 2) {
  console.error('Error: missing algo or hash argument');
  process.exit(1);
}

const [algo, hash] = positionals;
const urls = values.url || [];
const outputFile = values.output;

if (!outputFile) {
  // Go CLI defaults to stdout, but let's see if we strictly need it.
  // The example `get.rs` defaults to stdout.
  // The task says "passing the SDK as volume to download files".
  // Usually integration tests write to a file to verify existence/content, or just verify exit code.
  // I will default to stdout if no output is specified, but buffer handling might be tricky.
  // Actually, let's just write to stdout if no file.
}

async function main() {
  const servers = parseFetchurlServer(process.env.FETCHURL_SERVER ?? '');

  try {
    const data = await fetchurl({
      fetch: globalThis.fetch,
      servers,
      algo,
      hash,
      sourceUrls: urls,
    });

    if (outputFile) {
      fs.writeFileSync(outputFile, data);
    } else {
      process.stdout.write(data);
    }
  } catch (err) {
    console.error(`Error: ${err.message}`);
    process.exit(1);
  }
}

main();

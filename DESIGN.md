# Motivation

- Repeated downloads in CI environment
- Lack of standard in content addressable caching (we have pnpm and stuff but nothing standard that can be used by everyone)
- CI caching is mostly push based: build happens, artifacts are pushed based on a key
- Caching codes defined by intention instead of content (nix: hash of derivation, npm: cache of lockfile)

# Alternatives
- Backup the downloaded assets as workflow cache: looks very wasteful and loses value when anything changes (like the cache key is the hash of package-lock.json
- Caching proxy: too much hassle to setup a MITM proxy with a custom cert and force traffic through it
- Let it redownload assets every time

# Parts
- Client: the one that wants a file (npm, uv, hex, Nix)
- Server: the server proposed by this
- Upstream: the one that would directly serve the file to the client otherwise

# Scope
- Algoritms: sha1, sha256, sha512
- Only public data (no auth)
- Focus in caching package manager dependencies (ex: npm packages)

# Design

```
GET /api/fetchurl/sha256/e3b0... HTTP/1.1
X-Source-Urls: https://cdn1.com/file.tar.gz
X-Source-Urls: https://backup.org/archive.tgz
```

- Clients have the hash in hand
- Cliente have the source candidates in hand
- Clients ask the server to download for them: If server has then serves, if don't, multiwrite streams and abruptly aborts the connection if out of the happy path 
- Upstream must provide response size
- Server can preallocate a temp file so new consumers can read from ongoing transmissions and get notified on progress
- Successful transaction must have: right hash, known size
- It's possible to daisy chain servers and have a chain of cache
- Servers can evict any data at any time and have their own independent eviction policies to have the best cache hit vs resource usage tradeoff
- Data Dir: `/:algo/:shard/:hash`, shard = first two letters of the hash
- Environment passes the fetchurl server via the FETCHURL_SERVER environment variable, any clients supporting this spec must check for the environment variable. The value must be the full URL ready to append `/:algo/:hash`

# Challenges
- Adoption: implement logic on different clients to use a fetchurl server
- Adoption: CI providers exposing fetchurl servers in build environments
- Performance: efficient way to evict automatically data based on a policy, eviction is one atomic rm, keeping track of stuff is another problem

import argparse
import os
import sys

from . import fetch, UrllibFetcher, parse_fetchurl_server

def main():
    parser = argparse.ArgumentParser(description="Fetch a file using content-addressable storage")
    parser.add_argument("algo", help="Hash algorithm")
    parser.add_argument("hash", help="Expected hash")
    parser.add_argument("--url", action="append", default=[], help="Source URLs")
    parser.add_argument("-o", "--output", help="Output file")

    args = parser.parse_args()

    servers = parse_fetchurl_server(os.environ.get("FETCHURL_SERVER", ""))

    # Output handling
    if args.output:
        try:
            out = open(args.output, "wb")
        except OSError as e:
            sys.stderr.write(f"Error opening output file: {e}\n")
            sys.exit(1)
    else:
        out = sys.stdout.buffer

    try:
        fetch(
            fetcher=UrllibFetcher(),
            servers=servers,
            algo=args.algo,
            hash=args.hash,
            source_urls=args.url,
            out=out,
        )
    except Exception as e:
        sys.stderr.write(f"Error: {e}\n")
        sys.exit(1)
    finally:
        if args.output and out:
            out.close()

if __name__ == "__main__":
    main()

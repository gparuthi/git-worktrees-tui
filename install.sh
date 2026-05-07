#!/bin/bash
set -e
cd "$(dirname "$0")"
go build -trimpath -o wtree .
mkdir -p ~/.local/bin
mv wtree ~/.local/bin/wtree
echo "wtree installed to ~/.local/bin/wtree"

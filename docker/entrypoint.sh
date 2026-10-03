#!/bin/sh
set -eu

/atlas migrate apply --env local
exec /app/api

#! /bin/bash

jq -n '[inputs[]]' apis/*.json > api_kinds_v1.json

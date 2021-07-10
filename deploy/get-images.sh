#!/bin/bash

find deploy/all-in-one.yaml -type f -exec sed -rn 's/^\s*image: (.+?:.+)\s*$/\1/p' {} \;
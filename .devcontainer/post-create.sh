#!/bin/bash
set -e
set -x

# Remote podman by default
mkdir -p ~/.config/containers
cat <<'EOF' > ~/.config/containers/containers.conf
[engine]
remote = true
EOF

# medium docker images for act
mkdir -p ~/.config/act
cat <<'EOF' > ~/.config/act/actrc
-P ubuntu-latest=docker.io/catthehacker/ubuntu:act-latest
-P ubuntu-22.04=docker.io/catthehacker/ubuntu:act-22.04
-P ubuntu-20.04=docker.io/catthehacker/ubuntu:act-20.04
EOF

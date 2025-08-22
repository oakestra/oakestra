#! /bin/bash
set -e

sudo apt-get install g++ wget git
curl https://sh.rustup.rs -sSf | sh
. "$HOME/.cargo/env" 

# check if cmake installed
if ! command -v cmake &> /dev/null; then
    echo "CMake is not installed. Installing..."

    # Install latest cmake
    LATEST=$(curl -s https://api.github.com/repos/Kitware/CMake/releases/latest \
        | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' | sed 's/^v//')

    echo "Latest CMake version: $LATEST"

    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64)  PLATFORM="Linux-x86_64" ;;
        aarch64) PLATFORM="Linux-aarch64" ;;
        *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    INSTALLER="cmake-$LATEST-$PLATFORM.sh"
    URL="https://github.com/Kitware/CMake/releases/download/v$LATEST/$INSTALLER"

    TMPDIR=$(mktemp -d)
    cd "$TMPDIR"

    echo "Downloading $URL..."
    curl -LO "$URL"
    chmod +x "$INSTALLER"

    echo "Installing..."
    sudo ./"$INSTALLER" --skip-license --prefix=/usr/local

    cd /
    rm -rf "$TMPDIR"

    echo "CMake $LATEST installed successfully in /usr/local/bin"
else
    echo "CMake is already installed."
fi


# Installing wasm commands
git clone https://github.com/TintoEdoardo/wasm-migrate-commands.git --recursive
cd wasm-migrate-commands

cmake -S . -B build
cmake --build build

sudo mkdir -p /etc/oakestra/wasm/

sudo cp build/create_command /etc/oakestra/wasm/create_command
sudo cp build/start_command /etc/oakestra/wasm/start_command
sudo cp build/migrate_command /etc/oakestra/wasm/migrate_command

cd ../
rm -rf wasm-migrate-commands
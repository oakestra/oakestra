#!/bin/bash

if [ -z "$OAKESTRA_VERSION" ]; then
    OAKESTRA_VERSION=$(curl -s https://raw.githubusercontent.com/oakestra/oakestra/main/version.txt)
else 
    if [ "$OAKESTRA_VERSION" = "alpha" ]; then
        OAKESTRA_VERSION=alpha-$(curl -s https://raw.githubusercontent.com/oakestra/oakestra/develop/version.txt)
    fi
fi

ARCH=$(uname -m)

if [ "$ARCH" = "x86_64" ]; then
    ARCH=amd64
else
    if [ "$ARCH" = "aarch64" ]; then
        ARCH=arm64
    fi
fi
if [ "$ARCH" != "amd64" ] && [ "$ARCH" != "arm64" ]; then
    echo "Error: Unsupported architecture '${ARCH}'"
    exit 1
fi


echo Installing Oakestra Node Engine and Net Manager version $OAKESTRA_VERSION

rm NodeEngine_$ARCH.tar.gz 2> /dev/null
rm NetManager_$ARCH.tar.gz 2> /dev/null

wget -c https://github.com/oakestra/oakestra/releases/download/$OAKESTRA_VERSION/NodeEngine_$ARCH.tar.gz &&
    tar -xzf NodeEngine_$ARCH.tar.gz &&
    chmod +x install.sh &&
    ./install.sh $ARCH

if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve or install the Oakestra Node Engine."
        exit 1
fi

wget -c https://github.com/oakestra/oakestra-net/releases/download/$OAKESTRA_VERSION/NetManager_$ARCH.tar.gz &&
    tar -xzf NetManager_$ARCH.tar.gz &&
    chmod +x install.sh &&
    ./install.sh $ARCH

if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve or install the Oakestra Net Manager."
        exit 1
    fi

echo ✅ Installation complete

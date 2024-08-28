#!/bin/bash
if [ -z "$OAKESTRA_VERSION" ]; then
    OAKESTRA_VERSION=$(curl -s https://raw.githubusercontent.com/oakestra/oakestra/main/version.txt)
else 
    if [ "$OAKESTRA_VERSION" = "alpha" ]; then
        OAKESTRA_VERSION=alpha-$(curl -s https://raw.githubusercontent.com/oakestra/oakestra/develop/version.txt)
    fi
fi

echo Installing Oakestra Node Engine and Net Manager version $OAKESTRA_VERSION

rm NodeEngine_$(dpkg --print-architecture).tar.gz 2> /dev/null
rm NetManager_$(dpkg --print-architecture).tar.gz 2> /dev/null

wget -c https://github.com/oakestra/oakestra/releases/download/$OAKESTRA_VERSION/NodeEngine_$(dpkg --print-architecture).tar.gz && tar -xzf NodeEngine_$(dpkg --print-architecture).tar.gz && chmod +x install.sh && mv NodeEngine NodeEngine_$(dpkg --print-architecture) && ./install.sh $(dpkg --print-architecture)
if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve or install the Oakestra Node Engine."
        exit 1
fi

wget -c https://github.com/oakestra/oakestra-net/releases/download/$OAKESTRA_VERSION/NetManager_$(dpkg --print-architecture).tar.gz && tar -xzf NetManager_$(dpkg --print-architecture).tar.gz && chmod +x install.sh && ./install.sh $(dpkg --print-architecture)
if [ $? -ne 0 ]; then
        echo "Error: Failed to retrieve or install the Oakestra Net Manager."
        exit 1
    fi

echo ✅ Installation complete
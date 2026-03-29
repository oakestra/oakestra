#!/bin/bash
MOUNT_PATH="/var/lib/oakestra/csi/publish/csi.oakestra.io/hostpath/openclaw.dev.ollama.dec/0/my-named-volume"

echo "Checking mount status..."
if findmnt "$MOUNT_PATH" > /dev/null 2>&1; then
    echo "Mount is active. Attempting to unmount..."
    
    # Try lazy unmount if normal unmount fails
    if ! sudo umount "$MOUNT_PATH" 2>/dev/null; then
        echo "Normal unmount failed. Trying lazy unmount..."
        sudo umount -l "$MOUNT_PATH"
    fi
    
    sleep 1
    
    if findmnt "$MOUNT_PATH" > /dev/null 2>&1; then
        echo "Still mounted after lazy unmount. Checking for processes..."
        sudo fuser -vm "$MOUNT_PATH" 2>&1 || true
    else
        echo "Successfully unmounted!"
    fi
else
    echo "Not currently mounted"
fi

echo ""
echo "Now trying to remove directory..."
sudo rm -rf /var/lib/oakestra/csi/publish/csi.oakestra.io/hostpath/openclaw.dev.ollama.dec/

echo "Done!"

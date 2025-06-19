## Terraform Setup 

This directory contains Terraform scripts to set up a VMs with the NodeEngine pre-installed.

### Prerequisites

- [Terraform](https://www.terraform.io/downloads.html) installed
- qemu installed
- mkisofs installed -> `sudo apt install genisoimage`
- Necessary permissions to create and manage resources

### Usage

1. Initialize Terraform:

   ```bash
   terraform init
   ```

2. Create the VM running the Go Node Engine:

   ```bash
   terraform apply
   ```

3. After the VM is created, you can SSH into it using:

   ```bash
   ssh test@192.168.123.1
   ```

   and them run the Node Engine:

   ```bash
   sudo NodeEngine -a <IP of your cluster orchestrator> [-d]
   ```

4. Destroy the resources (when no longer needed):

   ```bash
   terraform destroy
   ```


## Troubleshooting

*Error*: `Error: error defining libvirt domain: operation failed: domain 'test' already exists with uuid 0771d977-7692-496d-b886-e7c65d44fbd5` 
- Run: `sudo virsh undefine --domain vm1` to remove the existing domain.

*Error*: `error creating libvirt domain: internal error: process exited while connecting to monitor: 2025-06-19T10:41:09.134982Z qemu-system-x86_64: -blockdev {"driver":"file","filename":"/opt/kvm/pool1/vm1","node-name":"libvirt-1-storage","auto-read-only":true,"discard":"unmap"}: Could not open '/opt/kvm/pool1/vm1': Permission denied``
- Set the following values to `/etc/libvirt/qemu.conf` file:
  ```ini
  user = "root"
  group = "root"
  security_driver = "none"
  ```
  Note: This is not recommended for production environments as it disables security features.
- Restart the libvirt service:
  ```bash
  sudo systemctl restart libvirtd
  ```

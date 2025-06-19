## Terraform Setup 

This directory contains Terraform scripts to set up different VMs to run the Go Node Engine.

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

2. Plan the deployment:

   ```bash
   terraform plan
   ```

3. Apply the changes:

   ```bash
   terraform apply
   ```

4. Destroy the resources (when no longer needed):

   ```bash
   terraform destroy
   ```


## Troubleshooting

*Error*: `Error: error defining libvirt domain: operation failed: domain 'test' already exists with uuid 0771d977-7692-496d-b886-e7c65d44fbd5` 
- Run: `sudo virsh undefine --domain vm1` to remove the existing domain.

*Error*: `error creating libvirt domain: internal error: process exited while connecting to monitor: 2025-06-19T10:41:09.134982Z qemu-system-x86_64: -blockdev {"driver":"file","filename":"/opt/kvm/pool1/vm1","node-name":"libvirt-1-storage","auto-read-only":true,"discard":"unmap"}: Could not open '/opt/kvm/pool1/vm1': Permission denied``
- Add the following to your `/etc/libvirt/qemu.conf` file:
  ```ini
  user = "root"
  group = "root"
  ```
- Reboot your system:
  ```bash
  sudo reboot -n
  ```
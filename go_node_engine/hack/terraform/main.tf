# Defining VM Volume
resource "libvirt_pool" "ubuntu" {
  name = "default"
  type = "dir"
  target {
    path = "/opt/kvm/pool1"
  }
}

### Network Configuration ###
resource "libvirt_network" "my_net" {
  name = "lv-net"
  mode = "nat"
  addresses = ["192.168.123.1/24"]
  autostart = true
}

### Image Configuration ###
resource "libvirt_volume" "vm1" {
  name = "vm1"
  pool = libvirt_pool.ubuntu.name
  source = "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64-disk-kvm.img"
  format = "qcow2"
}

resource "libvirt_domain" "vm1" {
  name = "vm1"
  description = "Test image"
  vcpu = 2
  memory = "2048"
  disk {
    volume_id = libvirt_volume.vm1.id
  }
  network_interface {
    network_id = libvirt_network.my_net.id
    wait_for_lease = true
  }
}
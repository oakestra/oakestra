# How to compile the NodeEngine ensuring compatibility with Wasmtime

## Step A: Skip this step if you don't need to compile Wasmtime to use newer versions or custom features

1. Ensure you have Rust installed:
```bash
rustc --version
cargo --version
```

2. Ensure you have the necessary dependencies to compile Wasmtime:
```bash
 sudo apt-get install gcc-aarch64-linux-gnu binutils-aarch64-linux-gnu cmake
```


3. Clone the Wasmtime repository:
```bash
git clone https://github.com/bytecodealliance/wasmtime.git
```

4. Build the Wasmtime C API:
```bash
cd wasmtime
cargo build --release -p wasmtime-c-api
```

5. Build the Wasmtime both for AMD64 and ARM64:
```bash
cargo clean
rustup target add aarch64-unknown-linux-gnu
export CC_aarch64_unknown_linux_gnu=aarch64-linux-gnu-gcc        
export CXX_aarch64_unknown_linux_gnu=aarch64-linux-gnu-g++
export AR_aarch64_unknown_linux_gnu=aarch64-linux-gnu-ar
export CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER=aarch64-linux-gnu-gcc
export RUSTFLAGS="-C linker=aarch64-linux-gnu-gcc -C link-arg=-lgcc_s"
cargo build --release -p wasmtime-c-api --target=aarch64-unknown-linux-gnu
unset CC_aarch64_unknown_linux_gnu CXX_aarch64_unknown_linux_gnu AR_aarch64_unknown_linux_gnu CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER RUSTFLAGS

export CC_x86_64_unknown_linux_gnu=gcc
export CXX_x86_64_unknown_linux_gnu=g++
export AR_x86_64_unknown_linux_gnu=ar
export CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_LINKER=gcc
export RUSTFLAGS="-C linker=gcc -C link-arg=-lgcc_s"
cargo build --release -p wasmtime-c-api --target=x86_64-unknown-linux-gnu
unset CC_aarch64_unknown_linux_gnu CXX_aarch64_unknown_linux_gnu AR_aarch64_unknown_linux_gnu CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER RUSTFLAGS

```

6. Copy the Wasmtime C API and Realease to the NodeEngine:
```bash
cp -rf $path_to_wasmtime/crates/c-api/include $path_to_oakestra/wasmtime-go/c-api
cp -rf $path_to_wasmtime/target/aarch64-unknown-linux-gnu $path_to_oakestra/wasmtime-go/target
cp -rf $path_to_wasmtime/target/x86_64-unknown-linux-gnu $path_to_oakestra/wasmtime-go/target
```

## Step B: Compile the NodeEngine

Run the following command to compile the NodeEngine:
```bash
cd $path_to_oakestra
./build.sh
```
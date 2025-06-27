# Installation

## Requirements

- **Go 1.23+** (for building from source)
- **Redis server** running on localhost:6379
- **NVIDIA GPUs** with nvidia-smi available
- **System access** to `/proc` filesystem or `ps` command for user detection

## Dependencies

### Go Dependencies
If building from source, Go dependencies are automatically managed:

```bash
go mod download
```

## Redis Setup

### Ubuntu/Debian
```bash
sudo apt update
sudo apt install redis-server
sudo systemctl start redis-server
sudo systemctl enable redis-server
```

### macOS (via Homebrew)
```bash
brew install redis
brew services start redis
```

### CentOS/RHEL/Fedora
```bash
sudo dnf install redis
sudo systemctl start redis
sudo systemctl enable redis
```

### Verify Redis Installation
```bash
redis-cli ping
# Should return: PONG
```

## NVIDIA Drivers

Ensure nvidia-smi is available:

```bash
nvidia-smi
# Should display GPU information
```

If not installed, install NVIDIA drivers for your system.

## Install canhazgpu

### Option 1: Build from Source (Recommended)
```bash
# Clone the repository
git clone https://github.com/russellb/canhazgpu.git
cd canhazgpu

# Build and install using Makefile
make install

# Optional: Install documentation dependencies for building docs
make docs-deps
```

### Option 2: Pre-built Binary
```bash
# Download pre-built binary (when available)
wget https://github.com/russellb/canhazgpu/releases/latest/download/canhazgpu
chmod +x canhazgpu

# Download bash completion script (optional)
wget https://raw.githubusercontent.com/russellb/canhazgpu/main/autocomplete_canhazgpu.sh

# Install system-wide
sudo cp canhazgpu /usr/local/bin/
sudo cp autocomplete_canhazgpu.sh /etc/bash_completion.d/
```

### Option 3: Local Installation
```bash
# Keep in local directory and add to PATH
export PATH="$PWD:$PATH"
```

## Bash Completion

The bash completion script provides tab completion for canhazgpu commands and options.

!!! important "Required for `canhazgpu run` Commands"
    Installing bash completion is required for proper tab completion when using commands with `canhazgpu run`. Without it, bash completion won't work for the commands you run after the `--` separator.

### Enable Completion

After installing the completion script to `/etc/bash_completion.d/`, enable it:

```bash
# Reload bash completion
source /etc/bash_completion

# Or restart your shell
exec bash
```

### Usage Examples

With bash completion enabled, you can use tab completion:

```bash
# Complete commands
canhazgpu <TAB>
# Shows: admin  release  reserve  run  status

# Complete options
canhazgpu run --<TAB>
# Shows: --gpus  --help

# Complete duration formats
canhazgpu reserve --duration <TAB>
# Shows common duration examples

# Complete commands after 'canhazgpu run --'
canhazgpu run --gpus 1 -- python <TAB>
# Shows available Python files and completion

# Complete program options
canhazgpu run --gpus 1 -- nvidia-smi --<TAB>
# Shows nvidia-smi options
```

### Manual Installation

If the automatic installation doesn't work, you can source the completion script manually:

```bash
# Add to your ~/.bashrc
echo "source /path/to/autocomplete_canhazgpu.sh" >> ~/.bashrc
source ~/.bashrc
```

## Verification

Test the installation:

```bash
canhazgpu --help
```

You should see the help output with available commands.

**Test bash completion** (if installed):
```bash
canhazgpu <TAB><TAB>
```

Should show available commands.

## Next Steps

Continue to the [Quick Start Guide](quickstart.md) to initialize and start using canhazgpu.
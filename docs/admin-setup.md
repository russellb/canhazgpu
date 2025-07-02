# Administration Setup

This guide covers setting up and configuring canhazgpu for production use in shared development environments.

## Initial System Setup

### 1. Install Dependencies

**System packages:**
```bash
# Ubuntu/Debian
sudo apt update
sudo apt install redis-server

# CentOS/RHEL/Fedora  
sudo dnf install redis

# macOS
brew install redis
```

**Verify NVIDIA tools:**
```bash
# Verify nvidia-smi is available
nvidia-smi
```

### 2. Redis Configuration

**Basic Redis setup:**
```bash
# Start Redis service
sudo systemctl start redis-server
sudo systemctl enable redis-server

# Verify Redis is running
redis-cli ping
# Should return: PONG
```

For Redis security and production configuration, refer to the official Redis documentation.

### 3. Install canhazgpu

**System-wide installation:**
```bash
# Download the latest release
wget https://github.com/russellb/canhazgpu/releases/latest/download/canhazgpu-linux-amd64.tar.gz

# Extract the archive
tar -xzf canhazgpu-linux-amd64.tar.gz

# Install the binary and bash completion
sudo cp canhazgpu /usr/local/bin/
sudo cp autocomplete_canhazgpu.sh /etc/bash_completion.d/

# Ensure proper permissions
sudo chmod 755 /usr/local/bin/canhazgpu
sudo chmod 644 /etc/bash_completion.d/autocomplete_canhazgpu.sh

# Clean up
rm canhazgpu-linux-amd64.tar.gz
```

**Verify installation:**
```bash
canhazgpu --help

# Test bash completion (optional)
# Start a new bash session and test:
# canhazgpu <TAB><TAB>
# Should show available commands
```

### 4. Application Configuration

Set up a default configuration for your team:

```bash
# Create a shared configuration file
sudo tee /usr/local/share/canhazgpu-default.yaml > /dev/null << 'EOF'
# Team default canhazgpu configuration
redis:
  host: "localhost"
  port: 6379
  db: 0

# Conservative memory threshold for shared systems  
memory:
  threshold: 512

# Default settings to encourage resource sharing
run:
  timeout: "4h"  # Prevent runaway processes

reserve:
  duration: "4h"  # Shorter default reservations

report:
  days: 30
EOF

# Make it readable by all users
sudo chmod 644 /usr/local/share/canhazgpu-default.yaml
```

**Usage:**
```bash
# Users can copy the default config to their home directory
cp /usr/local/share/canhazgpu-default.yaml ~/.canhazgpu.yaml

# Or use a specific config file for shared usage
canhazgpu --config /usr/local/share/canhazgpu-default.yaml status

# Users can customize their personal ~/.canhazgpu.yaml as needed
```

For detailed configuration options, see the [Configuration Guide](configuration.md).

## GPU Pool Initialization

### 1. Determine GPU Count
```bash
# Check available GPUs
nvidia-smi -L
# or
nvidia-smi --query-gpu=index,name --format=csv
```

### 2. Initialize the Pool
```bash
# Initialize with the correct number of GPUs
canhazgpu admin --gpus 8

# Verify initialization
canhazgpu status
```

### 3. Test Basic Functionality
```bash
# Test reservation
canhazgpu reserve --duration 5m

# Check status
canhazgpu status

# Test run command
canhazgpu run --gpus 1 -- nvidia-smi

# Release manual reservations
canhazgpu release
```

## Configuration Changes

### 1. Changing GPU Count
```bash
# When adding/removing GPUs from the system
canhazgpu admin --gpus 12 --force

# This will clear all existing reservations
# Notify users before making this change
```

### 2. Redis Migration
```bash
# Backup current state
redis-cli --rdb /backup/canhazgpu-backup.rdb

# After Redis migration/reinstall
redis-cli --rdb /backup/canhazgpu-backup.rdb | redis-cli --pipe

# Verify data integrity
canhazgpu status
```

### 3. System Upgrades
Before system upgrades:
```bash
# Warn users
wall "System maintenance in 15 minutes - GPU reservations will be cleared"

# Export current state for reference
canhazgpu status > /tmp/gpu-state-before-upgrade.txt

# After upgrade, reinitialize if needed
canhazgpu admin --gpus 8 --force
```

## Troubleshooting

### Common Issues

**Redis connection errors:**
```bash
# Check Redis status
sudo systemctl status redis-server

# Check Redis logs
sudo tail -f /var/log/redis/redis-server.log

# Test Redis connectivity
redis-cli ping
```

**Permission errors:**
```bash
# Check file permissions
ls -la /usr/local/bin/canhazgpu

# Check Redis permissions
sudo -u redis redis-cli ping
```

### Log Analysis
```bash
# System logs
sudo journalctl -u redis-server
sudo journalctl | grep canhazgpu

# Application logs (if configured)
tail -f /var/log/canhazgpu-health.log
tail -f /var/log/canhazgpu-stale.log
```

## Production Deployment Checklist

- [ ] Redis server installed and configured
- [ ] NVIDIA drivers working (`nvidia-smi` functional)
- [ ] Go 1.23+ installed
- [ ] canhazgpu installed system-wide
- [ ] Bash completion script installed
- [ ] GPU pool initialized with correct count
- [ ] Application configuration set up (see [Configuration Guide](configuration.md))
- [ ] Basic functionality tested
- [ ] Bash completion verified for users
- [ ] Security configuration applied
- [ ] Backup/recovery procedures documented
- [ ] User training materials prepared

## Next Steps

- **[Troubleshooting Guide](admin-troubleshooting.md)** - Common issues and solutions
- **[Architecture Overview](dev-architecture.md)** - Understand the system design
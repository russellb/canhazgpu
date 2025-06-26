# help text
.PHONY: help
help:
	@echo "make install   - Install the script to /usr/local/bin"

.PHONY: install
install:
	@echo "Installing script to /usr/local/bin"
	@sudo cp -v ./canhazgpu /usr/local/bin/canhazgpu

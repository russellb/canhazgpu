# help text
.PHONY: help
help:
	@echo "make install   - Install the script to /usr/local/bin"

.PHONY: install
install:
	@echo "Installing script to /usr/local/bin"
	@sudo cp -v ./canhazgpu /usr/local/bin/canhazgpu
	@sudo cp -v ./autocomplete_canhazgpu.sh /etc/bash_completion.d/autocomplete_canhazgpu.sh

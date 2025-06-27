# help text
.PHONY: help
help:
	@echo "make install      - Install the script to /usr/local/bin"
	@echo "make docs-deps    - Install documentation dependencies"
	@echo "make docs         - Build documentation with MkDocs"
	@echo "make docs-preview - Build and serve documentation locally"
	@echo "make docs-clean   - Clean documentation build files"

.PHONY: install
install:
	@echo "Installing script to /usr/local/bin"
	@sudo cp -v ./canhazgpu /usr/local/bin/canhazgpu
	@sudo cp -v ./autocomplete_canhazgpu.sh /etc/bash_completion.d/autocomplete_canhazgpu.sh

.PHONY: docs-deps
docs-deps:
	@echo "Installing documentation dependencies"
	@pip install -r requirements-docs.txt

.PHONY: docs
docs:
	@echo "Building documentation with MkDocs"
	@command -v mkdocs >/dev/null 2>&1 || { echo "Error: mkdocs not found. Install with: make docs-deps"; exit 1; }
	@mkdocs build || { echo "Error: MkDocs build failed. Install dependencies with: make docs-deps"; exit 1; }

.PHONY: docs-preview
docs-preview:
	@echo "Starting MkDocs development server"
	@command -v mkdocs >/dev/null 2>&1 || { echo "Error: mkdocs not found. Install with: make docs-deps"; exit 1; }
	@echo "Documentation will be available at: http://127.0.0.1:8000"
	@echo "Press Ctrl+C to stop the server"
	@mkdocs serve || { echo "Error: MkDocs serve failed. Install dependencies with: make docs-deps"; exit 1; }

.PHONY: docs-clean
docs-clean:
	@echo "Cleaning documentation build files"
	@rm -rf site/

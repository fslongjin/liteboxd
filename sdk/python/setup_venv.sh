#!/bin/bash
# Script to set up Python virtual environment for LiteBoxd SDK development

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="${SCRIPT_DIR}/.venv"

echo "Setting up Python virtual environment..."

# Check Python version
PYTHON_VERSION=$(python3 -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')
REQUIRED_VERSION="3.10"

if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$PYTHON_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
    echo "Error: Python $REQUIRED_VERSION or higher is required (found $PYTHON_VERSION)"
    exit 1
fi

echo "Python version: $PYTHON_VERSION"

# Create virtual environment if it doesn't exist
if [ ! -d "$VENV_DIR" ]; then
    echo "Creating virtual environment in $VENV_DIR..."
    python3 -m venv "$VENV_DIR"
else
    echo "Virtual environment already exists at $VENV_DIR"
fi

# Activate virtual environment
source "$VENV_DIR/bin/activate"

# Upgrade pip
echo "Upgrading pip..."
pip install --upgrade pip

# Install package with dev dependencies
echo "Installing liteboxd-sdk with dev dependencies..."
pip install -e ".[dev]"

echo ""
echo "Setup complete!"
echo ""
echo "To activate the virtual environment, run:"
echo "  source $VENV_DIR/bin/activate"
echo ""
echo "To run tests:"
echo "  pytest"
echo ""
echo "To run type checking:"
echo "  mypy src/liteboxd"

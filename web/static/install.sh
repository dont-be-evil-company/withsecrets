#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# GitHub repository details
REPO="dont-be-evil-company/withsecrets"
BINARY_NAME="ws"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to detect OS and architecture
detect_platform() {
    local os
    local arch

    # Detect OS
    case "$(uname -s)" in
        Linux*)     os="linux" ;;
        Darwin*)    os="darwin" ;;
        CYGWIN*|MINGW*|MSYS*) os="windows" ;;
        *)          print_error "Unsupported operating system: $(uname -s)"; exit 1 ;;
    esac

    # Detect architecture
    case "$(uname -m)" in
        x86_64|amd64)  arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        armv7l)        arch="armv7" ;;
        *)             print_error "Unsupported architecture: $(uname -m)"; exit 1 ;;
    esac

    # Handle special cases
    if [ "$os" = "darwin" ] && [ "$arch" = "arm64" ]; then
        arch="arm64"
    elif [ "$os" = "darwin" ] && [ "$arch" = "amd64" ]; then
        arch="amd64"
    fi

    echo "${os}-${arch}"
}

# Function to get latest release version
get_latest_version() {
    # GitHub automatically redirects /latest to the latest release
    # We can extract the version from the redirect URL
    local version
    local redirect_url

    if command -v curl >/dev/null 2>&1; then
        redirect_url=$(curl -s -I "https://github.com/${REPO}/releases/latest" | grep -i "location:" | sed 's/.*\/releases\/tag\///' | tr -d '\r')
    elif command -v wget >/dev/null 2>&1; then
        redirect_url=$(wget --spider -S "https://github.com/${REPO}/releases/latest" 2>&1 | grep -i "location:" | sed 's/.*\/releases\/tag\///' | tr -d '\r')
    else
        print_error "Neither curl nor wget is available. Please install one of them."
        exit 1
    fi

    if [ -z "$redirect_url" ]; then
        print_error "Failed to get latest version from GitHub"
        exit 1
    fi

    echo "$redirect_url"
}

# Function to download binary
download_binary() {
    local version="$1"
    local platform="$2"
    local download_url="https://github.com/${REPO}/releases/download/${version}/ws-${platform}"

    if [[ "$platform" = *"windows"* ]]; then
        download_url="${download_url}.exe"
    fi

    # Create temporary directory
    local temp_dir
    temp_dir=$(mktemp -d)
    local binary_path="${temp_dir}/${BINARY_NAME}"

    # Download binary using available downloader
    local download_success=false

    if command -v curl >/dev/null 2>&1; then
        if curl -L -o "$binary_path" "$download_url"; then
            download_success=true
        fi
    elif command -v wget >/dev/null 2>&1; then
        if wget -O "$binary_path" "$download_url"; then
            download_success=true
        fi
    fi

    if [ "$download_success" = false ]; then
        print_error "Failed to download binary from ${download_url}"
        rm -rf "$temp_dir"
        exit 1
    fi

    # Make binary executable (for Unix-like systems)
    if [[ "$platform" != *"windows"* ]]; then
        chmod +x "$binary_path"
    fi

    echo "$binary_path"
}

# Function to determine install location
get_install_location() {
    if [ "$(id -u)" -eq 0 ]; then
        # Running as root, install to system directory
        echo "/usr/local/bin/${BINARY_NAME}"
    else
        # Running as regular user, install to user's bin directory
        local user_bin="${HOME}/.local/bin"
        mkdir -p "$user_bin"
        echo "${user_bin}/${BINARY_NAME}"
    fi
}

# Function to detect shell and get RC file
detect_shell_rc() {
    local current_shell
    local shell_rc

    # Try to detect the actual shell being used
    if [ -n "$ZSH_VERSION" ]; then
        current_shell="zsh"
        shell_rc="${HOME}/.zshrc"
    elif [ -n "$BASH_VERSION" ]; then
        current_shell="bash"
        shell_rc="${HOME}/.bashrc"
    elif [ -n "$SHELL" ]; then
        # Fallback to SHELL environment variable
        case "$SHELL" in
            */zsh)  current_shell="zsh"; shell_rc="${HOME}/.zshrc" ;;
            */bash) current_shell="bash"; shell_rc="${HOME}/.bashrc" ;;
            *)      current_shell="unknown"; shell_rc="${HOME}/.profile" ;;
        esac
    else
        # Last resort fallback
        current_shell="unknown"
        shell_rc="${HOME}/.profile"
    fi

    echo "$shell_rc"
}

# Function to install binary
install_binary() {
    local source_path="$1"
    local install_path="$2"

    print_status "Installing ${BINARY_NAME} to ${install_path}..."

    # Create backup if binary already exists
    if [ -f "$install_path" ]; then
        local backup_path
        backup_path="${install_path}.backup.$(date +%Y%m%d_%H%M%S)"
        print_warning "Backing up existing binary to ${backup_path}"
        cp "$install_path" "$backup_path"
    fi

    # Install binary
    if cp "$source_path" "$install_path"; then
        print_success "${BINARY_NAME} installed successfully to ${install_path}"

        # Add to PATH if installing to user directory
        if [[ "$install_path" == *"/.local/bin"* ]]; then
            local shell_rc
            shell_rc=$(detect_shell_rc)
            local current_shell

            # Determine current shell for comparison
            if [ -n "$ZSH_VERSION" ]; then
                current_shell="zsh"
            elif [ -n "$BASH_VERSION" ]; then
                current_shell="bash"
            else
                current_shell="unknown"
            fi

            print_status "Detected shell: $current_shell, will update: $shell_rc"

            # Check if PATH already includes the user bin directory
            if ! grep -q "\.local/bin" "$shell_rc" 2>/dev/null; then
                print_status "Adding ${HOME}/.local/bin to PATH in ${shell_rc}"
                {
                  echo ""
                  echo "# Add local bin directory to PATH"
                  echo "export PATH=$HOME/.local/bin:$PATH"
                } >> "$shell_rc"
                print_warning "Please restart your shell or run 'source ${shell_rc}' to update PATH"
            else
                print_status "PATH already configured in ${shell_rc}"
            fi

            # Also try to update the current shell's RC file if different from detected
            if [ "$current_shell" != "unknown" ] && [ -n "$ZSH_VERSION" ] && [ "$shell_rc" != "${HOME}/.zshrc" ]; then
                print_warning "You're running zsh but the script detected a different shell"
                print_warning "You may also want to add this to your ~/.zshrc:"
                echo "export PATH=$HOME/.local/bin:$PATH" | sed 's/^/  /'
            elif [ "$current_shell" != "unknown" ] && [ -n "$BASH_VERSION" ] && [ "$shell_rc" != "${HOME}/.bashrc" ]; then
                print_warning "You're running bash but the script detected a different shell"
                print_warning "You may also want to add this to your ~/.bashrc:"
                echo "export PATH=$HOME/.local/bin:$PATH" | sed 's/^/  /'
            fi
        fi
    else
        print_error "Failed to install ${BINARY_NAME}"
        exit 1
    fi
}

# Function to verify installation
verify_installation() {
    local install_path="$1"

    if [ -f "$install_path" ] && [ -x "$install_path" ]; then
        print_success "Installation verified successfully!"
        print_status "You can now run: ${BINARY_NAME} --version"
    else
        print_error "Installation verification failed"
        exit 1
    fi
}

# Main installation function
main() {
    print_status "Installing ${BINARY_NAME}..."

    # Detect platform
    local platform
    platform=$(detect_platform)
    print_status "Detected platform: ${platform}"

    # Get latest version
    local version
    version=$(get_latest_version)
    print_status "Latest version: ${version}"

    print_status "Downloading ${BINARY_NAME} ${version} for ${platform}..."
    # Download binary
    local temp_binary
    temp_binary=$(download_binary "$version" "$platform")

    # Determine install location
    local install_path
    install_path=$(get_install_location)

    # Install binary
    install_binary "$temp_binary" "$install_path"

    # Clean up temporary files
    rm -rf "$(dirname "$temp_binary")"

    # Verify installation
    verify_installation "$install_path"

    print_success "${BINARY_NAME} installation completed successfully!"

    # Provide additional guidance for user installations
    if [[ "$install_path" == *"/.local/bin"* ]]; then
        local shell_rc
        shell_rc=$(detect_shell_rc)
        print_status "Installation completed! To use ${BINARY_NAME} from any location:"
        print_status "1. Restart your terminal, OR"
        print_status "2. Run: source $shell_rc"

        # Check if we're running in a different shell than detected
        if [ -n "$ZSH_VERSION" ] && [ "$shell_rc" != "${HOME}/.zshrc" ]; then
            print_warning "Note: You're running zsh but the script updated $shell_rc"
            print_warning "You may also want to add this to your ~/.zshrc:"
            echo "export PATH=$HOME/.local/bin:$PATH" | sed 's/^/  /'
        elif [ -n "$BASH_VERSION" ] && [ "$shell_rc" != "${HOME}/.bashrc" ]; then
            print_warning "Note: You're running bash but the script updated $shell_rc"
            print_warning "You may also want to add this to your ~/.bashrc:"
            echo "export PATH=$HOME/.local/bin:$PATH" | sed 's/^/  /'
        fi
    fi
}

# Check if running on Windows
if [[ "$(uname -s)" == *"MINGW"* ]] || [[ "$(uname -s)" == *"MSYS"* ]]; then
    print_error "This script is designed for Unix-like systems (Linux/macOS)"
    print_error "For Windows, please use the PowerShell installation script:"
    print_error "iwr https://withsecrets.com/install.ps1 -useb | iex"
    print_error "Or download and run: .\install.ps1"
    exit 1
fi

# Check if either curl or wget is available
if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
    print_error "Either curl or wget is required but neither is installed. Please install one of them first."
    exit 1
fi

# Run main function
main "$@"

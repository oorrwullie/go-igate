
#!/bin/bash

# Function to install AX.25 libraries, portaudio, rtl_fm, and multimon-ng on Debian or Ubuntu-based Linux
install_debian_or_ubuntu() {
    echo "Installing AX.25 libraries, portaudio, rtl_fm, and multimon-ng on Debian or Ubuntu-based Linux..."
    sudo apt-get update

    # Install AX.25 tools and libraries
    sudo apt-get install -y libax25 libax25-dev ax25-tools ax25-apps

    # Install portaudio
    sudo apt-get install -y portaudio19-dev

    # Install rtl_fm (part of rtl-sdr)
    sudo apt-get install -y rtl-sdr

    # Install multimon-ng
    sudo apt-get install -y multimon-ng

    if [ $? -eq 0 ]; then
        echo "AX.25 libraries, portaudio, rtl_fm, and multimon-ng successfully installed on Debian or Ubuntu-based Linux."
    else
        echo "Failed to install some components on Debian or Ubuntu-based Linux."
        exit 1
    fi
}

# Function to install AX.25 libraries, portaudio, rtl_fm, and multimon-ng on macOS using Homebrew
install_macos() {
    echo "Installing AX.25 libraries, portaudio, rtl_fm, and multimon-ng on macOS..."

    # Check if Homebrew is installed
    if ! command -v brew &> /dev/null; then
        echo "Homebrew is not installed. Installing Homebrew first..."
        /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    fi

    # Install AX.25 libraries using Homebrew
    brew tap hamlib/hamlib
    brew install hamlib

    # Install portaudio
    brew install portaudio

    # Install rtl_fm (part of rtl-sdr)
    brew install librtlsdr

    # Install multimon-ng
    brew install multimon-ng

    if [ $? -eq 0 ]; then
        echo "AX.25 libraries, portaudio, rtl_fm, and multimon-ng successfully installed on macOS."
    else
        echo "Failed to install some components on macOS."
        exit 1
    fi
}

# Determine the operating system
OS="$(uname -s)"

case "$OS" in
    Linux*)
        if [ -f /etc/debian_version ] || [ -f /etc/lsb-release ]; then
            install_debian_or_ubuntu
        else
            echo "Unsupported Linux distribution. This script only supports Debian or Ubuntu-based systems."
            exit 1
        fi
        ;;
    Darwin*)
        install_macos
        ;;
    *)
        echo "Unsupported operating system: $OS"
        exit 1
        ;;
esac

echo "Installation script completed."


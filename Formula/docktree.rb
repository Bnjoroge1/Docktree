class Docktree < Formula
  desc "Run Docker Compose services across multiple git worktrees without port conflicts"
  homepage "https://github.com/Bnjoroge1/Docktree"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.1.0/docktree_0.1.0_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER_ARM64_SHA256"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.1.0/docktree_0.1.0_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER_AMD64_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.1.0/docktree_0.1.0_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER_LINUX_ARM64_SHA256"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.1.0/docktree_0.1.0_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER_LINUX_AMD64_SHA256"
    end
  end

  def install
    bin.install "docktree"
  end

  test do
    assert_match "0.1.0", shell_output("#{bin}/docktree --version")
  end
end

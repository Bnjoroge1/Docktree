class Docktree < Formula
  desc "Run Docker Compose services across multiple git worktrees without port conflicts"
  homepage "https://github.com/Bnjoroge1/Docktree"
  version "0.6.5"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.5/docktree_0.6.5_darwin_arm64.tar.gz"
      sha256 "ba221ec01b6bfa3d295d46ddb5ba9ae24f47f392324e356d62bb1529defc23f8"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.5/docktree_0.6.5_darwin_amd64.tar.gz"
      sha256 "2c370d9c201a8b580c28059a933e7214c25e980af271debc11b9789fb5a82406"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.5/docktree_0.6.5_linux_arm64.tar.gz"
      sha256 "f39348af1e4c1a72712fb5c68fd3708ee95bc1b317ca4efbb879924827258a46"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.5/docktree_0.6.5_linux_amd64.tar.gz"
      sha256 "c817fa30946099a189b4b646f4b563be0d3d2d012d1cd08b93cfb0e4e7cb916a"
    end
  end

  def install
    bin.install "docktree"
  end

  test do
    assert_match "0.6.5", shell_output("#{bin}/docktree --version")
  end
end

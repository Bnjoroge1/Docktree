class Docktree < Formula
  desc "Run Docker Compose services across multiple git worktrees without port conflicts"
  homepage "https://github.com/Bnjoroge1/Docktree"
  version "0.6.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.0/docktree_0.6.0_darwin_arm64.tar.gz"
      sha256 "06f17f7d8d752f609bd10f5804e790e418fed75554e16d7de773d23b077f3a6d"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.0/docktree_0.6.0_darwin_amd64.tar.gz"
      sha256 "26892e90910d752534ba43bce1238483c850f573d3d754a24e09ceacdc92a6d2"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.0/docktree_0.6.0_linux_arm64.tar.gz"
      sha256 "bdb1384dd44a96cc446357629b7a637a7b593a5fa3d9e682a793309317bc82b2"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.0/docktree_0.6.0_linux_amd64.tar.gz"
      sha256 "b11fd3158f6528e8528c45710c0f49a951af5eec19e3990f257ea3178c9760c6"
    end
  end

  def install
    bin.install "docktree"
  end

  test do
    assert_match "0.6.0", shell_output("#{bin}/docktree --version")
  end
end

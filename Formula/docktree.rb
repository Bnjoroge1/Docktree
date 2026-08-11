class Docktree < Formula
  desc "Run Docker Compose services across multiple git worktrees without port conflicts"
  homepage "https://github.com/Bnjoroge1/Docktree"
  version "0.5.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.5.0/docktree_0.5.0_darwin_arm64.tar.gz"
      sha256 "98c4d8bdaed73106fd42a118978c9ba6569d2ba1c85605c3e357056f0bf6852d"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.5.0/docktree_0.5.0_darwin_amd64.tar.gz"
      sha256 "665065c74efffc5380b523c7a325f728c668da4fb7c85c7abeb1d7c52a450255"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.5.0/docktree_0.5.0_linux_arm64.tar.gz"
      sha256 "6dc51d67dd3255f5da01522fad1911aa860d92f0f93669e089ed04f7add16aaa"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.5.0/docktree_0.5.0_linux_amd64.tar.gz"
      sha256 "7aefe2c5a140d3a12276b7bc7ebd62d2bb74bc53df2b26fc3c4125145179323c"
    end
  end

  def install
    bin.install "docktree"
  end

  test do
    assert_match "0.5.0", shell_output("#{bin}/docktree --version")
  end
end

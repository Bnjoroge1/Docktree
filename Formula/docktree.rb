class Docktree < Formula
  desc "Run Docker Compose services across multiple git worktrees without port conflicts"
  homepage "https://github.com/Bnjoroge1/Docktree"
  version "0.1.8"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.1.8/docktree_0.1.8_darwin_arm64.tar.gz"
      sha256 "4907c8c92c0d16a1cad28bde437f730df2e1862493900621bfa63ab03102cec3"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.1.8/docktree_0.1.8_darwin_amd64.tar.gz"
      sha256 "44ab17e5b76b61709a88cbc116c288e36db638e109a4ef8c908d39b86f93d278"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.1.8/docktree_0.1.8_linux_arm64.tar.gz"
      sha256 "bf3391b10f062c61819fb01e9009083b92f6848ab65c20d33d94d15148e57668"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.1.8/docktree_0.1.8_linux_amd64.tar.gz"
      sha256 "aca19b3d58aea5310463c5d28ec4c2d98b666b6b12b2358bac3f64a0d25247d8"
    end
  end

  def install
    bin.install "docktree"
  end

  test do
    assert_match "0.1.8", shell_output("#{bin}/docktree --version")
  end
end

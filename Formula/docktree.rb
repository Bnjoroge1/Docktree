class Docktree < Formula
  desc "Run Docker Compose services across multiple git worktrees without port conflicts"
  homepage "https://github.com/Bnjoroge1/Docktree"
  version "0.6.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.0/docktree_0.6.0_darwin_arm64.tar.gz"
      sha256 "879e8efd3b554ece18beba3780be2446fd951a0fd29bf66235630be65a9c5d10"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.0/docktree_0.6.0_darwin_amd64.tar.gz"
      sha256 "3c7d797eb5ad73b79588d0258fc05cbcb5a8f9ee74afae8bc9be3568aac73913"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.0/docktree_0.6.0_linux_arm64.tar.gz"
      sha256 "27debbd3198d9cfd24b994e23f4af6a5b3570aab8cae8548627ddcdfc5878175"
    else
      url "https://github.com/Bnjoroge1/Docktree/releases/download/v0.6.0/docktree_0.6.0_linux_amd64.tar.gz"
      sha256 "6fead934643e9f5485e526eb969e39e5dcf5190b658d6ec37209eef5b88aaec6"
    end
  end

  def install
    bin.install "docktree"
  end

  test do
    assert_match "0.6.0", shell_output("#{bin}/docktree --version")
  end
end

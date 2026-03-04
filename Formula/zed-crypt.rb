class ZedCrypt < Formula
  desc "Transparent encryption for Zed editor using ccrypt"
  homepage "https://github.com/oglimmer/zed-crypt"
  url "https://github.com/oglimmer/zed-crypt/archive/refs/tags/v0.1.1.tar.gz"
  sha256 ""
  license "MIT"

  depends_on "go" => :build
  depends_on "ccrypt"

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w")
  end

  test do
    assert_match "zed-crypt", shell_output("#{bin}/zed-crypt 2>&1", 1)
  end
end

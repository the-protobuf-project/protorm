# typed: false
# frozen_string_literal: true

class ProtocGenProtorm < Formula
  desc "Protoc plugin generating Prisma, GORM, SQL & CSV schemas from Protobuf"
  homepage "https://github.com/oh-tarnished/protorm"
  url "https://github.com/oh-tarnished/protorm/archive/refs/tags/v0.1.0.tar.gz"
  # Update on first tagged release: shasum -a 256 of the source tarball above.
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  license "Apache-2.0"
  head "https://github.com/oh-tarnished/protorm.git", branch: "main"

  livecheck do
    url :stable
    regex(/^v?(\d+(?:\.\d+)+)$/i)
  end

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags:), "./plugin/cmd/protoc-gen-protorm"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/protoc-gen-protorm -version")
  end
end

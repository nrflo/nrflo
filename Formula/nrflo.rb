# Source build formula (includes systray support).
# For pre-built binaries: brew install nrflo/tap/nrflo
class Nrflo < Formula
  desc "Multi-workflow agent orchestration system"
  homepage "https://github.com/nrflo/nrflo"
  url "https://github.com/nrflo/nrflo/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PLACEHOLDER"
  license "MIT"

  depends_on "go" => :build
  depends_on "node" => :build

  def install
    # Build UI
    cd "ui" do
      system "npm", "install"
      system "npm", "run", "build"
    end

    # Copy UI dist to embed directory
    rm_rf "be/internal/static/dist"
    cp_r "ui/dist", "be/internal/static/dist"

    # Build and install Go binaries
    cd "be" do
      ldflags = %W[
        -s -w
        -X be/internal/cli.version=#{version}
      ]
      system "go", "build", *std_go_args(ldflags:, output: bin/"nrflo"), "./cmd/nrflo"
      system "go", "build", *std_go_args(ldflags:, output: bin/"nrflo_server"), "./cmd/server"
    end
  end

  def post_install
    (var/"nrflo").mkpath
  end

  def caveats
    <<~EOS
      Database is stored at ~/.nrflo/nrflo.data by default.
      Override with NRFLO_HOME environment variable.

      To start the server:
        nrflo_server serve

      To start as a background service:
        brew services start nrflo
    EOS
  end

  service do
    run [opt_bin/"nrflo_server", "serve"]
    keep_alive true
    log_path var/"log/nrflo/server.log"
    error_log_path var/"log/nrflo/server.log"
    working_dir var/"nrflo"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/nrflo version 2>&1", 0)
  end
end

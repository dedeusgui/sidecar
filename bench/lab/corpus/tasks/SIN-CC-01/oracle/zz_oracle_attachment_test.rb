# frozen_string_literal: true

require 'minitest/autorun'
require 'rack/mock'
require 'sinatra/base'

# Oculto: copiado para test/ somente após o agente terminar. Verifica cabeçalhos HTTP observáveis.
class ZzSinatraAttachmentOracleTest < Minitest::Test
  class App < Sinatra::Base
    get('/plain') { attachment 'report.txt'; 'x' }
    get('/quoted') { attachment 'a"b.txt'; 'x' }
    get('/unicode') { attachment 'relatório final.pdf'; 'x' }
    get('/file-ascii') { send_file File.expand_path(__FILE__), filename: 'file.txt' }
    get('/file-unicode') { send_file File.expand_path(__FILE__), filename: 'relatório.txt' }
  end

  def header(path)
    Rack::MockRequest.new(App).get(path).headers['Content-Disposition']
  end

  def test_ascii_attachment_header_is_unchanged
    assert_equal 'attachment; filename="report.txt"', header('/plain')
  end

  def test_quoted_ascii_attachment_header_is_unchanged
    assert_equal 'attachment; filename="a%22b.txt"', header('/quoted')
  end

  def test_ascii_send_file_header_is_unchanged
    assert_equal 'attachment; filename="file.txt"', header('/file-ascii')
  end

  def test_unicode_attachment_has_rfc5987_filename_star
    assert_includes header('/unicode'), "filename*=UTF-8''relat%C3%B3rio%20final.pdf"
  end

  def test_unicode_send_file_has_rfc5987_filename_star
    assert_includes header('/file-unicode'), "filename*=UTF-8''relat%C3%B3rio.txt"
  end
end

# frozen_string_literal: true

require 'minitest/autorun'
require 'rack/mock'
require 'sinatra/base'

# Oculto (holdout): copiado para test/ somente após o agente terminar. Verifica cabeçalhos HTTP observáveis.
class ZzSinatraContentTypeCharsetOracleTest < Minitest::Test
  class App < Sinatra::Base
    get('/plain-nil') { content_type 'text/plain', charset: nil; 'x' }
    get('/json-nil') { content_type 'application/json', charset: nil; 'x' }
    get('/plain-default') { content_type 'text/plain'; 'x' }
    get('/html-latin') { content_type 'text/html', charset: 'iso-8859-1'; 'x' }
    get('/html-nil') { content_type 'text/html', charset: nil; 'x' }
  end

  def content_type_of(path)
    Rack::MockRequest.new(App).get(path).headers['Content-Type']
  end

  def test_nil_charset_removes_charset_parameter
    assert_equal 'text/plain', content_type_of('/plain-nil')
  end

  def test_nil_charset_removes_charset_for_non_text_type
    assert_equal 'application/json', content_type_of('/json-nil')
  end

  def test_nil_charset_on_html_has_no_charset
    assert_equal 'text/html', content_type_of('/html-nil')
  end

  def test_default_charset_still_added_for_text
    assert_equal 'text/plain;charset=utf-8', content_type_of('/plain-default')
  end

  def test_explicit_charset_still_used
    assert_equal 'text/html;charset=iso-8859-1', content_type_of('/html-latin')
  end
end

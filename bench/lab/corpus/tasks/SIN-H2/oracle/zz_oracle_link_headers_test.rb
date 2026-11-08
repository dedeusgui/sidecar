# frozen_string_literal: true

require 'minitest/autorun'
require 'rack/mock'
require 'sinatra/base'
require 'sinatra/link_header'

# Oculto: copiado para test/ somente após o agente terminar. Verifica HTML e cabeçalho observáveis.
class ZzSinatraLinkHeadersOracleTest < Minitest::Test
  class App < Sinatra::Base
    helpers Sinatra::LinkHeader

    get('/stylesheet') { stylesheet '/style.css'; link_headers }
    get('/comma-url') { stylesheet '/assets/a,b.css'; link_headers }
    get('/semicolon-quoted') { link '/x', title: 'x; y, z'; link_headers }
    get('/comma-before-angle') { link '/x', title: 'a, <b>'; link_headers }
    get('/escaped-quote') { link '/x', title: 'diz "oi, tudo"; fim'; link_headers }
    get('/semicolon-url') { link '/a;b.css', rel: 'x'; link_headers }
    get('/two-prefetch') { prefetch '/a,b.js'; prefetch '/c'; link_headers }
    get('/raw-multi') do
      response['Link'] = '</a.css>; rel=stylesheet, </b,c.js>; rel=preload'
      link_headers
    end
    get('/raw-spaces') do
      response['Link'] = '<foo>     ;bar="baz"'
      link_headers
    end
    get('/block') { link_headers { link '/z' } }
    get('/empty') do
      response['Link'] = ''
      link_headers
    end
    get('/none') { link_headers }
    get('/header-only') { link '/a,b.css', title: 'x; y'; 'ok' }
    get('/link-return') { link('/q', rel: 'next') }
  end

  def get(path)
    Rack::MockRequest.new(App).get(path)
  end

  def test_stylesheet_tag_keeps_current_format
    assert_equal '<link href="/style.css" type="text/css" rel="stylesheet" />', get('/stylesheet').body
  end

  def test_comma_inside_url_is_not_a_separator
    assert_equal '<link href="/assets/a,b.css" type="text/css" rel="stylesheet" />', get('/comma-url').body
  end

  def test_semicolon_inside_quoted_param_is_kept
    assert_equal '<link href="/x" title="x; y, z" />', get('/semicolon-quoted').body
  end

  def test_comma_and_angle_inside_quoted_param_is_kept
    assert_equal '<link href="/x" title="a, <b>" />', get('/comma-before-angle').body
  end

  def test_escaped_quotes_and_separators_inside_quoted_param
    assert_equal '<link href="/x" title="diz \"oi, tudo\"; fim" />', get('/escaped-quote').body
  end

  def test_semicolon_inside_angle_brackets_is_part_of_url
    assert_equal '<link href="/a;b.css" rel="x" />', get('/semicolon-url').body
  end

  def test_several_entries_with_comma_in_url
    assert_equal '<link href="/a,b.js" rel="prefetch" /><link href="/c" rel="prefetch" />', get('/two-prefetch').body
  end

  def test_raw_header_with_spaces_and_comma_in_url
    assert_equal '<link href="/a.css" rel=stylesheet /><link href="/b,c.js" rel=preload />', get('/raw-multi').body
  end

  def test_raw_header_with_padding_keeps_format
    assert_equal '<link href="foo" bar="baz" />', get('/raw-spaces').body
  end

  def test_block_runs_before_collecting_links
    assert_includes get('/block').body, '<link href="/z"'
  end

  def test_empty_or_missing_link_header_gives_empty_string
    assert_equal '', get('/empty').body
    assert_equal '', get('/none').body
  end

  def test_link_header_value_written_by_helper_is_unchanged
    res = get('/header-only')
    assert_equal '</a,b.css>; title="x; y"', res.headers['Link']
  end

  def test_link_helper_return_value_is_unchanged
    assert_equal '<link href="/q" rel="next" />', get('/link-return').body
  end
end

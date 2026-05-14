// This is a generated file. Not intended for manual editing.
package com.github.alexpoliushkin.theater.thtrij.parser;

import com.intellij.lang.PsiBuilder;
import com.intellij.lang.PsiBuilder.Marker;
import static com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes.*;
import static com.intellij.lang.parser.GeneratedParserUtilBase.*;
import com.intellij.psi.tree.IElementType;
import com.intellij.lang.ASTNode;
import com.intellij.psi.tree.TokenSet;
import com.intellij.lang.PsiParser;
import com.intellij.lang.LightPsiParser;

@SuppressWarnings({"SimplifiableIfStatement", "UnusedAssignment"})
public class ThtrParser implements PsiParser, LightPsiParser {

  public ASTNode parse(IElementType root_, PsiBuilder builder_) {
    parseLight(root_, builder_);
    return builder_.getTreeBuilt();
  }

  public void parseLight(IElementType root_, PsiBuilder builder_) {
    boolean result_;
    builder_ = adapt_builder_(root_, builder_, this, null);
    Marker marker_ = enter_section_(builder_, 0, _COLLAPSE_, null);
    result_ = parse_root_(root_, builder_);
    exit_section_(builder_, 0, marker_, root_, result_, true, TRUE_CONDITION);
  }

  protected boolean parse_root_(IElementType root_, PsiBuilder builder_) {
    return parse_root_(root_, builder_, 0);
  }

  static boolean parse_root_(IElementType root_, PsiBuilder builder_, int level_) {
    return thtrFile(builder_, level_ + 1);
  }

  /* ********************************************************** */
  // ACT identifier_like line_tail
  public static boolean act_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "act_declaration")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, ACT_DECLARATION, "<act declaration>");
    result_ = consumeToken(builder_, ACT);
    pinned_ = result_; // pin = 1
    result_ = result_ && report_error_(builder_, identifier_like(builder_, level_ + 1));
    result_ = pinned_ && line_tail(builder_, level_ + 1) && result_;
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // AUTH declaration_tail
  public static boolean auth_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "auth_declaration")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, AUTH_DECLARATION, "<auth declaration>");
    result_ = consumeToken(builder_, AUTH);
    pinned_ = result_; // pin = 1
    result_ = result_ && declaration_tail(builder_, level_ + 1);
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // BACKEND declaration_tail
  public static boolean backend_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "backend_declaration")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, BACKEND_DECLARATION, "<backend declaration>");
    result_ = consumeToken(builder_, BACKEND);
    pinned_ = result_; // pin = 1
    result_ = result_ && declaration_tail(builder_, level_ + 1);
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // BAD_CHARACTER
  static boolean bad_character(PsiBuilder builder_, int level_) {
    return consumeToken(builder_, BAD_CHARACTER);
  }

  /* ********************************************************** */
  // BIND line_tail
  public static boolean bind_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "bind_statement")) return false;
    if (!nextTokenIs(builder_, BIND)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, BIND);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, BIND_STATEMENT, result_);
    return result_;
  }

  /* ********************************************************** */
  // CALL identifier_like line_tail
  public static boolean call_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "call_declaration")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, CALL_DECLARATION, "<call declaration>");
    result_ = consumeToken(builder_, CALL);
    pinned_ = result_; // pin = 1
    result_ = result_ && report_error_(builder_, identifier_like(builder_, level_ + 1));
    result_ = pinned_ && line_tail(builder_, level_ + 1) && result_;
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // CAPTURE_AUTH line_tail
  public static boolean capture_auth_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "capture_auth_statement")) return false;
    if (!nextTokenIs(builder_, CAPTURE_AUTH)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, CAPTURE_AUTH);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, CAPTURE_AUTH_STATEMENT, result_);
    return result_;
  }

  /* ********************************************************** */
  // stage_declaration
  //   | http_block
  //   | state_block
  //   | session_declaration
  //   | auth_declaration
  //   | identity_declaration
  //   | scenario_declaration
  //   | act_declaration
  //   | bind_statement
  //   | call_declaration
  //   | backend_declaration
  //   | record_declaration
  //   | pool_declaration
  //   | name_statement
  //   | do_statement
  //   | log_statement
  //   | expect_statement
  //   | eventually_statement
  //   | prop_statement
  //   | export_statement
  //   | transition_statement
  //   | dependency_statement
  //   | capture_auth_statement
  public static boolean declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "declaration")) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, DECLARATION, "<declaration>");
    result_ = stage_declaration(builder_, level_ + 1);
    if (!result_) result_ = http_block(builder_, level_ + 1);
    if (!result_) result_ = state_block(builder_, level_ + 1);
    if (!result_) result_ = session_declaration(builder_, level_ + 1);
    if (!result_) result_ = auth_declaration(builder_, level_ + 1);
    if (!result_) result_ = identity_declaration(builder_, level_ + 1);
    if (!result_) result_ = scenario_declaration(builder_, level_ + 1);
    if (!result_) result_ = act_declaration(builder_, level_ + 1);
    if (!result_) result_ = bind_statement(builder_, level_ + 1);
    if (!result_) result_ = call_declaration(builder_, level_ + 1);
    if (!result_) result_ = backend_declaration(builder_, level_ + 1);
    if (!result_) result_ = record_declaration(builder_, level_ + 1);
    if (!result_) result_ = pool_declaration(builder_, level_ + 1);
    if (!result_) result_ = name_statement(builder_, level_ + 1);
    if (!result_) result_ = do_statement(builder_, level_ + 1);
    if (!result_) result_ = log_statement(builder_, level_ + 1);
    if (!result_) result_ = expect_statement(builder_, level_ + 1);
    if (!result_) result_ = eventually_statement(builder_, level_ + 1);
    if (!result_) result_ = prop_statement(builder_, level_ + 1);
    if (!result_) result_ = export_statement(builder_, level_ + 1);
    if (!result_) result_ = transition_statement(builder_, level_ + 1);
    if (!result_) result_ = dependency_statement(builder_, level_ + 1);
    if (!result_) result_ = capture_auth_statement(builder_, level_ + 1);
    exit_section_(builder_, level_, marker_, result_, false, null);
    return result_;
  }

  /* ********************************************************** */
  // (identifier_like | COLON)? line_tail
  static boolean declaration_tail(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "declaration_tail")) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = declaration_tail_0(builder_, level_ + 1);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, null, result_);
    return result_;
  }

  // (identifier_like | COLON)?
  private static boolean declaration_tail_0(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "declaration_tail_0")) return false;
    declaration_tail_0_0(builder_, level_ + 1);
    return true;
  }

  // identifier_like | COLON
  private static boolean declaration_tail_0_0(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "declaration_tail_0_0")) return false;
    boolean result_;
    result_ = identifier_like(builder_, level_ + 1);
    if (!result_) result_ = consumeToken(builder_, COLON);
    return result_;
  }

  /* ********************************************************** */
  // DEPENDENCY line_tail
  public static boolean dependency_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "dependency_statement")) return false;
    if (!nextTokenIs(builder_, DEPENDENCY)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, DEPENDENCY);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, DEPENDENCY_STATEMENT, result_);
    return result_;
  }

  /* ********************************************************** */
  // DOTTED_REF
  public static boolean descriptor_ref(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "descriptor_ref")) return false;
    if (!nextTokenIs(builder_, DOTTED_REF)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, DOTTED_REF);
    exit_section_(builder_, marker_, DESCRIPTOR_REF, result_);
    return result_;
  }

  /* ********************************************************** */
  // DO line_tail
  public static boolean do_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "do_statement")) return false;
    if (!nextTokenIs(builder_, DO)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, DO);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, DO_STATEMENT, result_);
    return result_;
  }

  /* ********************************************************** */
  // EVENTUALLY line_tail
  public static boolean eventually_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "eventually_statement")) return false;
    if (!nextTokenIs(builder_, EVENTUALLY)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, EVENTUALLY);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, EVENTUALLY_STATEMENT, result_);
    return result_;
  }

  /* ********************************************************** */
  // EXPECT line_tail
  public static boolean expect_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "expect_statement")) return false;
    if (!nextTokenIs(builder_, EXPECT)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, EXPECT);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, EXPECT_STATEMENT, result_);
    return result_;
  }

  /* ********************************************************** */
  // EXPORT identifier_like line_tail
  public static boolean export_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "export_statement")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, EXPORT_STATEMENT, "<export statement>");
    result_ = consumeToken(builder_, EXPORT);
    pinned_ = result_; // pin = 1
    result_ = result_ && report_error_(builder_, identifier_like(builder_, level_ + 1));
    result_ = pinned_ && line_tail(builder_, level_ + 1) && result_;
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // GENERATE_REF
  public static boolean generator_call(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "generator_call")) return false;
    if (!nextTokenIs(builder_, GENERATE_REF)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, GENERATE_REF);
    exit_section_(builder_, marker_, GENERATOR_CALL, result_);
    return result_;
  }

  /* ********************************************************** */
  // HTTP line_tail
  public static boolean http_block(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "http_block")) return false;
    if (!nextTokenIs(builder_, HTTP)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, HTTP);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, HTTP_BLOCK, result_);
    return result_;
  }

  /* ********************************************************** */
  // IDENTIFIER | DOTTED_REF | DOLLAR_REF
  static boolean identifier_like(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "identifier_like")) return false;
    boolean result_;
    result_ = consumeToken(builder_, IDENTIFIER);
    if (!result_) result_ = consumeToken(builder_, DOTTED_REF);
    if (!result_) result_ = consumeToken(builder_, DOLLAR_REF);
    return result_;
  }

  /* ********************************************************** */
  // IDENTITY declaration_tail
  public static boolean identity_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "identity_declaration")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, IDENTITY_DECLARATION, "<identity declaration>");
    result_ = consumeToken(builder_, IDENTITY);
    pinned_ = result_; // pin = 1
    result_ = result_ && declaration_tail(builder_, level_ + 1);
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // declaration | line_atom
  static boolean item(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "item")) return false;
    boolean result_;
    result_ = declaration(builder_, level_ + 1);
    if (!result_) result_ = line_atom(builder_, level_ + 1);
    return result_;
  }

  /* ********************************************************** */
  // WHEN | EVERY | REPEATABLE | TRUE | FALSE | NULL | HAS | NO | ITEM | ALL | ITEMS | ENTRY | KEY | LACKS | IS | BETWEEN | AND | WHERE | MATCHES | CONTAINS | ASSERT
  static boolean keyword_atom(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "keyword_atom")) return false;
    boolean result_;
    result_ = consumeToken(builder_, WHEN);
    if (!result_) result_ = consumeToken(builder_, EVERY);
    if (!result_) result_ = consumeToken(builder_, REPEATABLE);
    if (!result_) result_ = consumeToken(builder_, TRUE);
    if (!result_) result_ = consumeToken(builder_, FALSE);
    if (!result_) result_ = consumeToken(builder_, NULL);
    if (!result_) result_ = consumeToken(builder_, HAS);
    if (!result_) result_ = consumeToken(builder_, NO);
    if (!result_) result_ = consumeToken(builder_, ITEM);
    if (!result_) result_ = consumeToken(builder_, ALL);
    if (!result_) result_ = consumeToken(builder_, ITEMS);
    if (!result_) result_ = consumeToken(builder_, ENTRY);
    if (!result_) result_ = consumeToken(builder_, KEY);
    if (!result_) result_ = consumeToken(builder_, LACKS);
    if (!result_) result_ = consumeToken(builder_, IS);
    if (!result_) result_ = consumeToken(builder_, BETWEEN);
    if (!result_) result_ = consumeToken(builder_, AND);
    if (!result_) result_ = consumeToken(builder_, WHERE);
    if (!result_) result_ = consumeToken(builder_, MATCHES);
    if (!result_) result_ = consumeToken(builder_, CONTAINS);
    if (!result_) result_ = consumeToken(builder_, ASSERT);
    return result_;
  }

  /* ********************************************************** */
  // generator_call | selector_call | object_value | list_value | descriptor_ref | identifier_like | keyword_atom | literal | punctuation | bad_character
  static boolean line_atom(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "line_atom")) return false;
    boolean result_;
    result_ = generator_call(builder_, level_ + 1);
    if (!result_) result_ = selector_call(builder_, level_ + 1);
    if (!result_) result_ = object_value(builder_, level_ + 1);
    if (!result_) result_ = list_value(builder_, level_ + 1);
    if (!result_) result_ = descriptor_ref(builder_, level_ + 1);
    if (!result_) result_ = identifier_like(builder_, level_ + 1);
    if (!result_) result_ = keyword_atom(builder_, level_ + 1);
    if (!result_) result_ = literal(builder_, level_ + 1);
    if (!result_) result_ = punctuation(builder_, level_ + 1);
    if (!result_) result_ = bad_character(builder_, level_ + 1);
    return result_;
  }

  /* ********************************************************** */
  // line_atom*
  static boolean line_tail(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "line_tail")) return false;
    while (true) {
      int pos_ = current_position_(builder_);
      if (!line_atom(builder_, level_ + 1)) break;
      if (!empty_element_parsed_guard_(builder_, "line_tail", pos_)) break;
    }
    return true;
  }

  /* ********************************************************** */
  // LIST
  public static boolean list_value(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "list_value")) return false;
    if (!nextTokenIs(builder_, LIST)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, LIST);
    exit_section_(builder_, marker_, LIST_VALUE, result_);
    return result_;
  }

  /* ********************************************************** */
  // STRING | NUMBER | DURATION
  static boolean literal(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "literal")) return false;
    boolean result_;
    result_ = consumeToken(builder_, STRING);
    if (!result_) result_ = consumeToken(builder_, NUMBER);
    if (!result_) result_ = consumeToken(builder_, DURATION);
    return result_;
  }

  /* ********************************************************** */
  // LOG identifier_like line_tail
  public static boolean log_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "log_statement")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, LOG_STATEMENT, "<log statement>");
    result_ = consumeToken(builder_, LOG);
    pinned_ = result_; // pin = 1
    result_ = result_ && report_error_(builder_, identifier_like(builder_, level_ + 1));
    result_ = pinned_ && line_tail(builder_, level_ + 1) && result_;
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // NAME line_tail
  public static boolean name_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "name_statement")) return false;
    if (!nextTokenIs(builder_, NAME)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, NAME);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, NAME_STATEMENT, result_);
    return result_;
  }

  /* ********************************************************** */
  // OBJECT
  public static boolean object_value(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "object_value")) return false;
    if (!nextTokenIs(builder_, OBJECT)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, OBJECT);
    exit_section_(builder_, marker_, OBJECT_VALUE, result_);
    return result_;
  }

  /* ********************************************************** */
  // POOL declaration_tail
  public static boolean pool_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "pool_declaration")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, POOL_DECLARATION, "<pool declaration>");
    result_ = consumeToken(builder_, POOL);
    pinned_ = result_; // pin = 1
    result_ = result_ && declaration_tail(builder_, level_ + 1);
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // PROP identifier_like line_tail
  public static boolean prop_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "prop_statement")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, PROP_STATEMENT, "<prop statement>");
    result_ = consumeToken(builder_, PROP);
    pinned_ = result_; // pin = 1
    result_ = result_ && report_error_(builder_, identifier_like(builder_, level_ + 1));
    result_ = pinned_ && line_tail(builder_, level_ + 1) && result_;
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // L_PAREN | R_PAREN | L_BRACE | R_BRACE | L_BRACKET | R_BRACKET | COMMA | COLON | DOT | EQUALS | EQEQ | ARROW | PIPE | BANG | GT | GTE | LT | LTE
  static boolean punctuation(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "punctuation")) return false;
    boolean result_;
    result_ = consumeToken(builder_, L_PAREN);
    if (!result_) result_ = consumeToken(builder_, R_PAREN);
    if (!result_) result_ = consumeToken(builder_, L_BRACE);
    if (!result_) result_ = consumeToken(builder_, R_BRACE);
    if (!result_) result_ = consumeToken(builder_, L_BRACKET);
    if (!result_) result_ = consumeToken(builder_, R_BRACKET);
    if (!result_) result_ = consumeToken(builder_, COMMA);
    if (!result_) result_ = consumeToken(builder_, COLON);
    if (!result_) result_ = consumeToken(builder_, DOT);
    if (!result_) result_ = consumeToken(builder_, EQUALS);
    if (!result_) result_ = consumeToken(builder_, EQEQ);
    if (!result_) result_ = consumeToken(builder_, ARROW);
    if (!result_) result_ = consumeToken(builder_, PIPE);
    if (!result_) result_ = consumeToken(builder_, BANG);
    if (!result_) result_ = consumeToken(builder_, GT);
    if (!result_) result_ = consumeToken(builder_, GTE);
    if (!result_) result_ = consumeToken(builder_, LT);
    if (!result_) result_ = consumeToken(builder_, LTE);
    return result_;
  }

  /* ********************************************************** */
  // RECORD declaration_tail
  public static boolean record_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "record_declaration")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, RECORD_DECLARATION, "<record declaration>");
    result_ = consumeToken(builder_, RECORD);
    pinned_ = result_; // pin = 1
    result_ = result_ && declaration_tail(builder_, level_ + 1);
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // !(STAGE | HTTP | STATE | SESSION | AUTH | IDENTITY | SCENARIO | ACT | BIND | CALL | BACKEND | RECORD | POOL | NAME | DO | LOG | EXPECT | EVENTUALLY | PROP | EXPORT | ON | DEPENDENCY | CAPTURE_AUTH)
  static boolean recover_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "recover_declaration")) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_, level_, _NOT_);
    result_ = !recover_declaration_0(builder_, level_ + 1);
    exit_section_(builder_, level_, marker_, result_, false, null);
    return result_;
  }

  // STAGE | HTTP | STATE | SESSION | AUTH | IDENTITY | SCENARIO | ACT | BIND | CALL | BACKEND | RECORD | POOL | NAME | DO | LOG | EXPECT | EVENTUALLY | PROP | EXPORT | ON | DEPENDENCY | CAPTURE_AUTH
  private static boolean recover_declaration_0(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "recover_declaration_0")) return false;
    boolean result_;
    result_ = consumeToken(builder_, STAGE);
    if (!result_) result_ = consumeToken(builder_, HTTP);
    if (!result_) result_ = consumeToken(builder_, STATE);
    if (!result_) result_ = consumeToken(builder_, SESSION);
    if (!result_) result_ = consumeToken(builder_, AUTH);
    if (!result_) result_ = consumeToken(builder_, IDENTITY);
    if (!result_) result_ = consumeToken(builder_, SCENARIO);
    if (!result_) result_ = consumeToken(builder_, ACT);
    if (!result_) result_ = consumeToken(builder_, BIND);
    if (!result_) result_ = consumeToken(builder_, CALL);
    if (!result_) result_ = consumeToken(builder_, BACKEND);
    if (!result_) result_ = consumeToken(builder_, RECORD);
    if (!result_) result_ = consumeToken(builder_, POOL);
    if (!result_) result_ = consumeToken(builder_, NAME);
    if (!result_) result_ = consumeToken(builder_, DO);
    if (!result_) result_ = consumeToken(builder_, LOG);
    if (!result_) result_ = consumeToken(builder_, EXPECT);
    if (!result_) result_ = consumeToken(builder_, EVENTUALLY);
    if (!result_) result_ = consumeToken(builder_, PROP);
    if (!result_) result_ = consumeToken(builder_, EXPORT);
    if (!result_) result_ = consumeToken(builder_, ON);
    if (!result_) result_ = consumeToken(builder_, DEPENDENCY);
    if (!result_) result_ = consumeToken(builder_, CAPTURE_AUTH);
    return result_;
  }

  /* ********************************************************** */
  // SCENARIO identifier_like line_tail
  public static boolean scenario_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "scenario_declaration")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, SCENARIO_DECLARATION, "<scenario declaration>");
    result_ = consumeToken(builder_, SCENARIO);
    pinned_ = result_; // pin = 1
    result_ = result_ && report_error_(builder_, identifier_like(builder_, level_ + 1));
    result_ = pinned_ && line_tail(builder_, level_ + 1) && result_;
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // FIELD | DECODE | PATH | PICK | REGEXP
  public static boolean selector_call(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "selector_call")) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, SELECTOR_CALL, "<selector call>");
    result_ = consumeToken(builder_, FIELD);
    if (!result_) result_ = consumeToken(builder_, DECODE);
    if (!result_) result_ = consumeToken(builder_, PATH);
    if (!result_) result_ = consumeToken(builder_, PICK);
    if (!result_) result_ = consumeToken(builder_, REGEXP);
    exit_section_(builder_, level_, marker_, result_, false, null);
    return result_;
  }

  /* ********************************************************** */
  // SESSION declaration_tail
  public static boolean session_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "session_declaration")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, SESSION_DECLARATION, "<session declaration>");
    result_ = consumeToken(builder_, SESSION);
    pinned_ = result_; // pin = 1
    result_ = result_ && declaration_tail(builder_, level_ + 1);
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // STAGE identifier_like line_tail
  public static boolean stage_declaration(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "stage_declaration")) return false;
    boolean result_, pinned_;
    Marker marker_ = enter_section_(builder_, level_, _NONE_, STAGE_DECLARATION, "<stage declaration>");
    result_ = consumeToken(builder_, STAGE);
    pinned_ = result_; // pin = 1
    result_ = result_ && report_error_(builder_, identifier_like(builder_, level_ + 1));
    result_ = pinned_ && line_tail(builder_, level_ + 1) && result_;
    exit_section_(builder_, level_, marker_, result_, pinned_, ThtrParser::recover_declaration);
    return result_ || pinned_;
  }

  /* ********************************************************** */
  // STATE line_tail
  public static boolean state_block(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "state_block")) return false;
    if (!nextTokenIs(builder_, STATE)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, STATE);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, STATE_BLOCK, result_);
    return result_;
  }

  /* ********************************************************** */
  // item*
  static boolean thtrFile(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "thtrFile")) return false;
    while (true) {
      int pos_ = current_position_(builder_);
      if (!item(builder_, level_ + 1)) break;
      if (!empty_element_parsed_guard_(builder_, "thtrFile", pos_)) break;
    }
    return true;
  }

  /* ********************************************************** */
  // ON line_tail
  public static boolean transition_statement(PsiBuilder builder_, int level_) {
    if (!recursion_guard_(builder_, level_, "transition_statement")) return false;
    if (!nextTokenIs(builder_, ON)) return false;
    boolean result_;
    Marker marker_ = enter_section_(builder_);
    result_ = consumeToken(builder_, ON);
    result_ = result_ && line_tail(builder_, level_ + 1);
    exit_section_(builder_, marker_, TRANSITION_STATEMENT, result_);
    return result_;
  }

}

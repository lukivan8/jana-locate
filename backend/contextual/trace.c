#define _GNU_SOURCE
#include "libpostal.h"
#include <string.h>
static void quoted(const char *s) {
 putchar('"');
 for (const unsigned char *p=(const unsigned char*)s;*p;p++) {
  if (*p=='"'||*p=='\\') {putchar('\\');putchar(*p);}
  else if (*p<32) printf("\\u%04x",*p);
  else putchar(*p);
 }
 putchar('"');
}
int main(void) {
 char *dir="/models/libpostal";
 if (!libpostal_setup_datadir(dir)||!libpostal_setup_parser_datadir(dir)) return 2;
 libpostal_address_parser_options_t options=libpostal_get_address_parser_default_options();options.country="kz";
 char *line=NULL;size_t size=0;ssize_t length;
 while ((length=getdelim(&line,&size,'\0',stdin))!=-1) {
  libpostal_address_parser_response_t *r=libpostal_parse_address(line,options);
  if (!r) return 3;
  putchar('[');
  for (size_t i=0;i<r->num_components;i++) {
   if(i) putchar(',');printf("{\"label\":");quoted(r->labels[i]);printf(",\"value\":");quoted(r->components[i]);putchar('}');
  }
  puts("]");fflush(stdout);libpostal_address_parser_response_destroy(r);
 }
 free(line);return 0;
}

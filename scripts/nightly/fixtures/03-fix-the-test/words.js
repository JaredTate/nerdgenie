// Counts the words in a sentence. Every run of white space separates two words.
export function countWords(sentence) {
  return sentence.split(" ").length;
}
